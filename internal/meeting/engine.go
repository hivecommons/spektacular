package meeting

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// Engine is the meeting state machine. It is single-threaded and takes the
// current time on every call, so a test drives it with a fake clock and a
// replayed transcript and gets the same intent stream every run. Session
// wires it to stdin, stdout, a clock and a Decider.
type Engine struct {
	s     Settings
	det   *Detector
	title string
	ctx   []ContextFile
	log   io.Writer

	idPrefix  string
	nextQ     int
	nextR     int
	window    []Line
	lastFinal time.Time
	// newSinceTurn is set by a final segment and cleared when a turn
	// decision is taken, so a pause is acted on once.
	newSinceTurn bool
	// addressedDue is set by a final line that names Spek; it triggers a
	// decision without waiting for a pause.
	addressedDue bool
	// lastAddressed is whether the latest final segment named Spek; say is
	// only allowed while it holds.
	lastAddressed bool
	inFlight      bool

	notes     map[string][]string
	questions []Question
	askTimes  []time.Time
	research  []ResearchItem
	offers    []offer
	chats     []string

	lastHeartbeat time.Time
	started       bool
	ended         bool
}

type offer struct {
	query string
	at    time.Time
}

// Options configure an engine.
type Options struct {
	Settings Settings
	Title    string
	Context  []ContextFile
	// IDPrefix is put in every qid and rid ("q-<prefix>1"), so ids from two
	// runs for the same meeting cannot collide. Empty in tests.
	IDPrefix string
	Log      io.Writer
}

// NewEngine returns an engine for one meeting.
func NewEngine(o Options) *Engine {
	if o.Log == nil {
		o.Log = io.Discard
	}
	return &Engine{
		s:        o.Settings,
		det:      NewDetector(o.Settings.Names),
		title:    o.Title,
		ctx:      o.Context,
		log:      o.Log,
		idPrefix: o.IDPrefix,
		notes:    map[string][]string{},
	}
}

func (e *Engine) logf(format string, args ...any) {
	fmt.Fprintf(e.log, "spektacular meeting: "+format+"\n", args...)
}

// Ended reports whether an end event has been seen.
func (e *Engine) Ended() bool { return e.ended }

// Start begins the meeting and returns the first heartbeat, so hive knows
// at once that the child is alive.
func (e *Engine) Start(now time.Time) []Intent {
	e.started = true
	e.lastHeartbeat = now
	return []Intent{{Type: IntentHeartbeat}}
}

// Tick advances time: it emits a heartbeat when one is due and closes
// offers nobody answered.
func (e *Engine) Tick(now time.Time) []Intent {
	var out []Intent
	if e.started && now.Sub(e.lastHeartbeat) >= e.s.Heartbeat {
		e.lastHeartbeat = now
		out = append(out, Intent{Type: IntentHeartbeat})
	}
	kept := e.offers[:0]
	for _, o := range e.offers {
		if now.Sub(o.at) < e.s.OfferTTL {
			kept = append(kept, o)
		}
	}
	e.offers = kept
	return out
}

// HandleLine parses and handles one stdin line. A malformed line is logged
// and skipped.
func (e *Engine) HandleLine(line []byte, now time.Time) []Intent {
	if strings.TrimSpace(string(line)) == "" {
		return nil
	}
	ev, err := ParseEvent(line)
	if err != nil {
		e.logf("skipping malformed input line: %v", err)
		return nil
	}
	return e.HandleEvent(ev, now)
}

// HandleEvent handles one parsed event.
func (e *Engine) HandleEvent(ev Event, now time.Time) []Intent {
	if e.ended {
		return nil
	}
	switch ev.Type {
	case EventSegment:
		if !ev.Final {
			e.logf("skipping non-final segment seq=%d", ev.Seq)
			return nil
		}
		return e.onSpoken(personName(ev.Speaker), ev.Text, false, now)
	case EventChat:
		if e.isSelf(ev.Author) {
			return nil
		}
		return e.onSpoken(personName(ev.Author), ev.Text, true, now)
	case EventAnswer:
		e.onAnswer(ev)
		return nil
	case EventResearchUpdate:
		return e.onResearchUpdate(ev)
	case EventEnd:
		e.ended = true
		return nil
	default:
		e.logf("skipping unknown event type %q", ev.Type)
		return nil
	}
}

func personName(p *Person) string {
	if p == nil || strings.TrimSpace(p.Name) == "" {
		return "unknown"
	}
	return strings.TrimSpace(p.Name)
}

// isSelf reports whether a chat line is Spek's own post echoed back.
func (e *Engine) isSelf(p *Person) bool {
	if p == nil {
		return false
	}
	n := strings.ToLower(strings.TrimSpace(p.Name))
	for _, name := range e.s.Names {
		if n == name {
			return true
		}
	}
	return false
}

// onSpoken handles one final segment or chat line.
func (e *Engine) onSpoken(who, text string, chat bool, now time.Time) []Intent {
	text = strings.TrimSpace(text)
	e.window = append(e.window, Line{Speaker: who, Text: text, Chat: chat})
	if over := len(e.window) - e.s.WindowSegments; over > 0 {
		e.window = append([]Line(nil), e.window[over:]...)
	}
	addressed := e.det.Addressed(text)
	if !chat {
		e.lastFinal = now
		e.newSinceTurn = true
		e.lastAddressed = addressed
	}

	// An explicit request is research at once.
	if q, ok := e.det.ExplicitResearch(text); ok {
		return e.startResearch(q, "requested in the meeting by "+who)
	}
	// A reply to an open offer settles it.
	if len(e.offers) > 0 {
		switch e.det.Reply(text) {
		case 1:
			o := e.offers[len(e.offers)-1]
			e.offers = e.offers[:len(e.offers)-1]
			return e.startResearch(o.query, "offered by Spek and agreed by "+who)
		case -1:
			e.offers = e.offers[:len(e.offers)-1]
			return nil
		}
	}
	var out []Intent
	// A passing mention becomes an offer.
	if q, ok := ImplicitResearch(text); ok {
		out = append(out, e.offer(q, now)...)
	}
	if addressed {
		e.addressedDue = true
	}
	return out
}

func (e *Engine) knownResearch(q string) bool {
	for _, r := range e.research {
		if Similar(r.Query, q) {
			return true
		}
	}
	return false
}

func (e *Engine) startResearch(q, reason string) []Intent {
	if e.knownResearch(q) {
		e.logf("not repeating research already requested: %q", q)
		return nil
	}
	// Starting research settles any open offer for the same topic.
	kept := e.offers[:0]
	for _, o := range e.offers {
		if !Similar(o.query, q) {
			kept = append(kept, o)
		}
	}
	e.offers = kept
	e.nextR++
	rid := fmt.Sprintf("r-%s%d", e.idPrefix, e.nextR)
	e.research = append(e.research, ResearchItem{RID: rid, Query: q, Status: "requested"})
	return []Intent{{Type: IntentResearch, RID: rid, Query: truncate(q), Reason: truncate(reason)}}
}

func (e *Engine) offer(q string, now time.Time) []Intent {
	if e.knownResearch(q) {
		return nil
	}
	for _, o := range e.offers {
		if Similar(o.query, q) {
			return nil
		}
	}
	e.offers = append(e.offers, offer{query: q, at: now})
	return []Intent{{Type: IntentChatPost, Text: truncate(fmt.Sprintf("Want me to research %q? Reply yes and I'll start.", q))}}
}

func (e *Engine) onAnswer(ev Event) {
	for i := range e.questions {
		if e.questions[i].QID == ev.QID {
			e.questions[i].Answer = strings.TrimSpace(ev.Text)
			e.questions[i].By = ev.By
			return
		}
	}
	e.logf("answer for unknown qid %q ignored", ev.QID)
}

func (e *Engine) onResearchUpdate(ev Event) []Intent {
	for i := range e.research {
		r := &e.research[i]
		if r.RID != ev.RID {
			continue
		}
		wasDone := r.Status == ResearchDone
		r.Status = ev.Status
		if ev.Summary != "" {
			r.Summary = strings.TrimSpace(ev.Summary)
		}
		if ev.URL != "" {
			r.URL = ev.URL
		}
		if ev.Status == ResearchDone && !wasDone && r.Summary != "" {
			text := fmt.Sprintf("Research %q: %s", r.Query, r.Summary)
			if r.URL != "" {
				text += " (" + r.URL + ")"
			}
			return e.note(SectionNotes, text)
		}
		return nil
	}
	e.logf("research_update for unknown rid %q ignored", ev.RID)
	return nil
}

// note records and emits one notes line unless it repeats one already in
// its section.
func (e *Engine) note(section, text string) []Intent {
	text = truncate(strings.TrimSpace(text))
	if text == "" {
		return nil
	}
	n := Normalize(text)
	for _, have := range e.notes[section] {
		if Normalize(have) == n {
			return nil
		}
	}
	e.notes[section] = append(e.notes[section], text)
	return []Intent{{Type: IntentNotesAppend, Section: section, Text: text}}
}

// asksLeft is how many asks the rolling ten-minute budget still allows.
func (e *Engine) asksLeft(now time.Time) int {
	kept := e.askTimes[:0]
	for _, t := range e.askTimes {
		if now.Sub(t) < AskWindow {
			kept = append(kept, t)
		}
	}
	e.askTimes = kept
	return e.s.MaxAsksPer10m - len(e.askTimes)
}

// Due returns a turn decision request when one should be taken now: Spek
// was addressed, or a pause followed new final speech. At most one decision
// is in flight; Apply completes it.
func (e *Engine) Due(now time.Time) (Request, bool) {
	if e.inFlight || e.ended {
		return Request{}, false
	}
	trigger := ""
	switch {
	case e.addressedDue:
		trigger = TriggerAddressed
	case e.newSinceTurn && now.Sub(e.lastFinal) >= e.s.Pause:
		trigger = TriggerPause
	default:
		return Request{}, false
	}
	e.addressedDue = false
	e.newSinceTurn = false
	e.inFlight = true
	req := e.request(KindTurn, now)
	req.Trigger = trigger
	req.AskAllowed = trigger == TriggerPause && e.asksLeft(now) > 0
	req.SayAllowed = e.lastAddressed
	return req, true
}

// SummaryRequest is the closing decision.
func (e *Engine) SummaryRequest(now time.Time) Request {
	return e.request(KindSummary, now)
}

func (e *Engine) request(kind string, now time.Time) Request {
	notes := map[string][]string{}
	for k, v := range e.notes {
		notes[k] = append([]string(nil), v...)
	}
	return Request{
		Kind:      kind,
		Title:     e.title,
		Context:   e.ctx,
		Window:    append([]Line(nil), e.window...),
		Notes:     notes,
		Questions: append([]Question(nil), e.questions...),
		Research:  append([]ResearchItem(nil), e.research...),
	}
}

// Limits on what one decision may emit, so a talkative model cannot flood
// the meeting.
const (
	maxAsksPerDecision     = 1
	maxNotesPerDecision    = 6
	maxChatPerDecision     = 1
	maxOffersPerDecision   = 1
	maxSummaryNotesAtClose = 12
)

// Apply completes a turn decision: it validates the model's proposals and
// applies the rules (asks only at pauses and within budget, no repeated
// questions, say only while the latest segment addresses Spek, model
// research only as an offer). A failed decision emits nothing.
func (e *Engine) Apply(req Request, resp Response, err error, now time.Time) []Intent {
	e.inFlight = false
	if err != nil {
		e.logf("decision failed (%s): %v", req.Trigger, err)
		return nil
	}
	if e.ended {
		// Notes still count toward the closing summary; nothing else does.
		var out []Intent
		for _, n := range limitNotes(resp.Notes, maxNotesPerDecision) {
			out = append(out, e.note(n.Section, n.Text)...)
		}
		return out
	}
	var out []Intent

	asked := 0
	for _, a := range resp.Asks {
		if asked >= maxAsksPerDecision {
			break
		}
		if req.Trigger != TriggerPause {
			e.logf("dropping ask outside a pause: %q", a.Text)
			continue
		}
		if in, ok := e.ask(a, now); ok {
			out = append(out, in)
			asked++
		}
	}

	for _, n := range limitNotes(resp.Notes, maxNotesPerDecision) {
		out = append(out, e.note(n.Section, n.Text)...)
	}

	posted := 0
	for _, c := range resp.Chat {
		c = truncate(strings.TrimSpace(c))
		if c == "" || posted >= maxChatPerDecision || e.chatted(c) {
			continue
		}
		e.chats = append(e.chats, c)
		out = append(out, Intent{Type: IntentChatPost, Text: c})
		posted++
	}

	if s := strings.TrimSpace(resp.Say); s != "" {
		if req.SayAllowed && e.lastAddressed {
			out = append(out, Intent{Type: IntentSay, Text: truncate(s)})
		} else {
			e.logf("dropping say: the latest segment did not address Spek by name")
		}
	}

	offered := 0
	for _, r := range resp.Research {
		if offered >= maxOffersPerDecision {
			break
		}
		if q := cleanQuery(r.Query); q != "" {
			if in := e.offer(q, now); len(in) > 0 {
				out = append(out, in...)
				offered++
			}
		}
	}
	return out
}

func limitNotes(notes []ProposedNote, n int) []ProposedNote {
	var out []ProposedNote
	for _, note := range notes {
		switch note.Section {
		case SectionDecisions, SectionActions, SectionQuestions, SectionNotes:
		default:
			continue
		}
		if len(out) == n {
			break
		}
		out = append(out, note)
	}
	return out
}

func (e *Engine) chatted(c string) bool {
	for _, have := range e.chats {
		if Normalize(have) == Normalize(c) {
			return true
		}
	}
	return false
}

// ask applies the ask rules to one proposed question.
func (e *Engine) ask(a ProposedAsk, now time.Time) (Intent, bool) {
	text := truncate(strings.TrimSpace(a.Text))
	if text == "" {
		return Intent{}, false
	}
	if a.DuplicateOf != "" {
		e.logf("dropping ask the model marked a repeat of %s: %q", a.DuplicateOf, text)
		return Intent{}, false
	}
	for _, q := range e.questions {
		if Similar(q.Text, text) {
			e.logf("dropping repeated ask (same as %s): %q", q.QID, text)
			return Intent{}, false
		}
	}
	if e.asksLeft(now) <= 0 {
		e.logf("dropping ask: %d asks in the last 10 minutes already", e.s.MaxAsksPer10m)
		return Intent{}, false
	}
	e.nextQ++
	qid := fmt.Sprintf("q-%s%d", e.idPrefix, e.nextQ)
	e.questions = append(e.questions, Question{QID: qid, Text: text})
	e.askTimes = append(e.askTimes, now)
	return Intent{Type: IntentAsk, QID: qid, Text: text}, true
}

// Finish closes the meeting: it records any notes the summary decision
// proposed and emits the summary section (decisions, actions with owners
// where stated, open questions, research). A failed summary decision still
// yields the summary from what was recorded during the meeting.
func (e *Engine) Finish(resp Response, err error) []Intent {
	e.ended = true
	e.inFlight = false
	var out []Intent
	overview := ""
	if err != nil {
		e.logf("summary decision failed; summarising recorded notes only: %v", err)
	} else {
		for _, n := range limitNotes(resp.Notes, maxSummaryNotesAtClose) {
			out = append(out, e.note(n.Section, n.Text)...)
		}
		overview = strings.TrimSpace(resp.Summary)
	}
	for _, chunk := range chunkText(e.summaryText(overview), MaxTextChars) {
		out = append(out, Intent{Type: IntentNotesAppend, Section: SectionSummary, Text: chunk})
	}
	return out
}

func (e *Engine) summaryText(overview string) string {
	var b strings.Builder
	title := e.title
	if title == "" {
		title = "Meeting"
	}
	b.WriteString(title + " summary\n")
	if overview != "" {
		b.WriteString("\n" + overview + "\n")
	}
	list := func(heading string, items []string) {
		if len(items) == 0 {
			return
		}
		b.WriteString("\n" + heading + ":\n")
		for _, it := range items {
			b.WriteString("- " + it + "\n")
		}
	}
	list("Decisions", e.notes[SectionDecisions])
	list("Actions", e.notes[SectionActions])

	var open []string
	for _, q := range e.questions {
		if q.Answer == "" {
			open = append(open, q.Text)
		}
	}
	for _, q := range e.notes[SectionQuestions] {
		dup := false
		for _, o := range open {
			if Similar(o, q) {
				dup = true
				break
			}
		}
		if !dup {
			open = append(open, q)
		}
	}
	list("Open questions", open)

	var answered []string
	for _, q := range e.questions {
		if q.Answer != "" {
			a := q.Text + " " + q.Answer
			if q.By != "" {
				a += " (" + q.By + ")"
			}
			answered = append(answered, a)
		}
	}
	list("Answered", answered)

	var research []string
	for _, r := range e.research {
		line := fmt.Sprintf("%s: %s", r.Query, r.Status)
		if r.Summary != "" {
			line += ", " + r.Summary
		}
		research = append(research, line)
	}
	list("Research", research)

	if overview == "" && len(e.notes[SectionDecisions]) == 0 && len(e.notes[SectionActions]) == 0 && len(open) == 0 && len(answered) == 0 && len(research) == 0 {
		b.WriteString("\nNo decisions, actions or open questions were recorded.\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// chunkText splits text at line boundaries into pieces of at most max
// characters, cutting a single over-long line where it must.
func chunkText(text string, max int) []string {
	var out []string
	var cur []rune
	for _, line := range strings.SplitAfter(text, "\n") {
		r := []rune(line)
		if len(cur)+len(r) > max && len(cur) > 0 {
			out = append(out, strings.TrimRight(string(cur), "\n"))
			cur = nil
		}
		for len(r) > max {
			out = append(out, string(r[:max]))
			r = r[max:]
		}
		cur = append(cur, r...)
	}
	if s := strings.TrimRight(string(cur), "\n"); strings.TrimSpace(s) != "" {
		out = append(out, s)
	}
	return out
}

func truncate(s string) string {
	r := []rune(s)
	if len(r) <= MaxTextChars {
		return s
	}
	return string(r[:MaxTextChars-1]) + "…"
}
