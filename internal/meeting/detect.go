package meeting

import (
	"regexp"
	"strings"
)

// Detector holds the text rules that run on every final line without a
// model call: who is being addressed, explicit and implicit research
// requests, and replies to an offer.
type Detector struct {
	address  []*regexp.Regexp
	explicit []*regexp.Regexp
	leadName *regexp.Regexp
}

// fillers may come before a name at the start of an utterance.
const fillers = `(?:hey|hi|ok|okay|so|and|um|uh|right|alright|thanks|thank you)`

// researchVerbs are the verbs that ask for research.
const researchVerbs = `(?:research|look up|look into|find out(?: about)?|dig into|investigate|search for|check on)`

// NewDetector builds the rules for the given names ("hive", "spek").
func NewDetector(names []string) *Detector {
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, regexp.QuoteMeta(n))
	}
	alt := `(?:` + strings.Join(quoted, "|") + `)`
	ask := `(?:(?:can|could|would|will) you |please |go |and )*`
	return &Detector{
		address: []*regexp.Regexp{
			// "hive, ...", "hey spek ..." at the start.
			regexp.MustCompile(`(?i)^\s*(?:` + fillers + `[,\s]+)*` + alt + `\b`),
			// "... hive, ..." or "... spek: ..." anywhere.
			regexp.MustCompile(`(?i)\b` + alt + `\s*[,:]`),
			// "..., hive?" at the end.
			regexp.MustCompile(`(?i),\s*` + alt + `\s*[?!.]*\s*$`),
		},
		explicit: []*regexp.Regexp{
			// "hive, research X", "spek can you look into X".
			regexp.MustCompile(`(?i)\b` + alt + `\b[,:]?\s+` + ask + researchVerbs + `\s+(.+)`),
			// "can you look up X", "could you research X".
			regexp.MustCompile(`(?i)\b(?:can|could|would|will) you (?:please )?(?:go )?` + researchVerbs + `\s+(.+)`),
			// "go find out X", "please research X".
			regexp.MustCompile(`(?i)(?:^|[,.;]\s*|\b(?:and|so)\s+)(?:go|please)\s+(?:and\s+)?` + researchVerbs + `\s+(.+)`),
		},
		leadName: regexp.MustCompile(`(?i)^\s*(?:` + fillers + `[,\s]+)*` + alt + `\b[\s,:!.]*`),
	}
}

var implicitResearch = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:we|someone|somebody|i|you guys)\s+(?:really\s+|probably\s+|also\s+|maybe\s+)?(?:should|need to|ought to|must|have to|might want to|want to)\s+(?:really\s+|probably\s+|also\s+|maybe\s+)?(?:look into|research|find out(?: about)?|investigate|dig into|figure out|check(?: on| out)?)\s+(.+)`),
	regexp.MustCompile(`(?i)\bit(?:'d| would) be (?:good|worth|nice|useful) to (?:know|find out(?: about)?|look into|research|check)\s+(.+)`),
	regexp.MustCompile(`(?i)\b(?:worth|let's)\s+(?:looking into|researching|finding out|look into|research|find out)\s+(.+)`),
}

// Addressed reports whether text speaks to Spek by one of its names.
func (d *Detector) Addressed(text string) bool {
	for _, re := range d.address {
		if re.MatchString(text) {
			return true
		}
	}
	return false
}

// ExplicitResearch returns the query of an explicit research request
// ("hive, research X", "can you look up X", "go find out X").
func (d *Detector) ExplicitResearch(text string) (string, bool) {
	for _, re := range d.explicit {
		if m := re.FindStringSubmatch(text); m != nil {
			if q := cleanQuery(m[1]); q != "" {
				return q, true
			}
		}
	}
	return "", false
}

// ImplicitResearch returns the query of a passing mention ("we should look
// into X"), which becomes an offer rather than research.
func ImplicitResearch(text string) (string, bool) {
	for _, re := range implicitResearch {
		if m := re.FindStringSubmatch(text); m != nil {
			if q := cleanQuery(m[1]); q != "" {
				return q, true
			}
		}
	}
	return "", false
}

var affirmatives = []string{
	"yes", "yeah", "yep", "yup", "sure", "ok", "okay", "please", "absolutely",
	"definitely", "go ahead", "do it", "sounds good", "go for it", "please do",
	"that would be great", "that'd be great", "good idea", "great idea", "agreed", "why not",
}

var negatives = []string{
	"no", "nope", "nah", "not now", "don't", "dont", "do not", "skip", "never mind",
	"nevermind", "not yet", "no thanks", "no need", "not needed", "hold off",
}

// Reply classifies a short reply to an offer: +1 agrees, -1 declines, 0 is
// neither. Only a reply that opens with the agreement or refusal counts, so
// "yes, but the real problem is..." agrees and a long unrelated line does not.
func (d *Detector) Reply(text string) int {
	t := strings.ToLower(d.leadName.ReplaceAllString(text, ""))
	t = strings.TrimSpace(strings.Trim(t, " ,.!?"))
	if t == "" {
		return 0
	}
	if len(strings.Fields(t)) > maxReplyWords {
		return 0
	}
	for _, n := range negatives {
		if hasWordPrefix(t, n) {
			return -1
		}
	}
	for _, a := range affirmatives {
		if hasWordPrefix(t, a) {
			return 1
		}
	}
	return 0
}

// maxReplyWords bounds how long a line can be and still be a reply to an
// offer.
const maxReplyWords = 12

func hasWordPrefix(t, p string) bool {
	if !strings.HasPrefix(t, p) {
		return false
	}
	if len(t) == len(p) {
		return true
	}
	c := t[len(p)]
	return !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9')
}

var trailingQuery = regexp.MustCompile(`(?i)(?:[\s,]+(?:for us|for me|for the team|please|thanks|thank you|real quick|quickly|if you can|when you get a chance|after the meeting))+\s*$`)
var leadingQuery = regexp.MustCompile(`(?i)^(?:about|on|into)\s+`)

func cleanQuery(q string) string {
	q = strings.TrimSpace(q)
	q = strings.TrimRight(q, " .!?,;:")
	for {
		next := strings.TrimRight(trailingQuery.ReplaceAllString(q, ""), " .!?,;:")
		if next == q {
			break
		}
		q = next
	}
	q = leadingQuery.ReplaceAllString(q, "")
	q = strings.TrimSpace(q)
	if len([]rune(q)) > maxQueryChars {
		q = string([]rune(q)[:maxQueryChars])
	}
	return q
}

// maxQueryChars bounds a research query lifted from speech.
const maxQueryChars = 300

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

// Normalize lower-cases text and reduces it to words, the form questions and
// queries are compared in.
func Normalize(text string) string {
	return strings.TrimSpace(nonWord.ReplaceAllString(strings.ToLower(text), " "))
}

var stopwords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true, "was": true, "were": true,
	"do": true, "does": true, "did": true, "we": true, "you": true, "i": true, "it": true,
	"to": true, "of": true, "for": true, "on": true, "in": true, "and": true, "or": true,
	"what": true, "which": true, "who": true, "how": true, "when": true, "where": true, "why": true,
	"should": true, "would": true, "could": true, "can": true, "will": true, "be": true,
	"that": true, "this": true, "there": true, "our": true, "us": true, "any": true, "about": true,
	"so": true, "then": true, "just": true, "now": true, "again": true, "actually": true, "ok": true, "okay": true,
}

// Similar reports whether two questions are near-duplicates: identical once
// normalized, or sharing most of their content words. The model's own
// duplicate_of judgement covers rephrasings this misses.
func Similar(a, b string) bool {
	na, nb := Normalize(a), Normalize(b)
	if na == "" || nb == "" {
		return false
	}
	if na == nb {
		return true
	}
	wa, wb := contentWords(na), contentWords(nb)
	if len(wa) == 0 || len(wb) == 0 {
		return false
	}
	inter := 0
	for w := range wa {
		if wb[w] {
			inter++
		}
	}
	union := len(wa) + len(wb) - inter
	return float64(inter)/float64(union) >= similarThreshold
}

// similarThreshold is the Jaccard overlap of content words above which two
// questions are treated as the same question.
const similarThreshold = 0.7

func contentWords(norm string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.Fields(norm) {
		if !stopwords[w] {
			out[w] = true
		}
	}
	return out
}
