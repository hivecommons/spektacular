package runstats

import (
	"time"

	"github.com/hivecommons/spektacular/internal/agentusage"
)

// Token use statuses and the reasons it is unknown.
const (
	TokensKnown   = "known"
	TokensUnknown = "unknown"

	// ReasonNotRead: the report was built without session data.
	ReasonNotRead = "not_read"
	// ReasonAgentUnsupported: the run's agent is not Claude Code.
	ReasonAgentUnsupported = "agent_unsupported"
	// ReasonSessionNotFound: no session ran the run's own commands.
	ReasonSessionNotFound = "session_not_found"
	// The other reasons are the session reader's problem codes:
	// agentusage.ProblemDirMissing, ProblemUnreadable and
	// ProblemUnrecognised.
)

// Why session overhead is unknown.
const (
	OverheadSeveralRuns     = "several_runs"
	OverheadNoTotals        = "no_totals"
	OverheadBelowTranscript = "totals_below_transcript"
)

// TokenInput is what a report needs to give token use: the agent sessions
// that could cover the run, the problems met reading them, the price
// table, and every run in the logs, to split a session that drove several.
// Dir is the sessions directory searched, for messages.
type TokenInput struct {
	Sessions []*agentusage.Session
	Problems []agentusage.Problem
	Prices   Prices
	AllRuns  []*Run
	Dir      string

	// driven memoises, per transcript, the runs it drove.
	driven map[*agentusage.Transcript][]*Run
}

// TokenFigure is the token use of one part of a run: the four token types,
// the cost of the priced part, whether every token was priced (and the
// models that were not), and working time per thousand output tokens.
//
// Cost is rounded to 6 decimals; it is null when the figure holds tokens
// and none of them are priced, and 0 only when it holds no tokens.
// WorkingMSPer1kOutput is integer milliseconds, null with no output tokens
// or for a figure with no time of its own.
type TokenFigure struct {
	Input                int64    `json:"input"`
	Output               int64    `json:"output"`
	CacheRead            int64    `json:"cache_read"`
	CacheWrite           int64    `json:"cache_write"`
	Cost                 *float64 `json:"cost"`
	CostComplete         bool     `json:"cost_complete"`
	UnpricedModels       []string `json:"unpriced_models"`
	WorkingMSPer1kOutput *int64   `json:"working_ms_per_1k_output"`
}

// SessionRef names one transcript that drove the run.
type SessionRef struct {
	SessionID  string `json:"session_id"`
	Transcript string `json:"transcript"`
	File       string `json:"file"`
}

// ModelTokens is one model's tokens over the run, with their cost (null
// when the model has no price).
type ModelTokens struct {
	Model      string   `json:"model"`
	Input      int64    `json:"input"`
	Output     int64    `json:"output"`
	CacheRead  int64    `json:"cache_read"`
	CacheWrite int64    `json:"cache_write"`
	Cost       *float64 `json:"cost"`
}

// UnmatchedHelper is a helper transcript launched during the run that no
// recorded helper span matches. Its tokens still count, in the step and
// task it was launched in.
type UnmatchedHelper struct {
	Name       string       `json:"name"`
	Step       *string      `json:"step"`
	TaskID     *string      `json:"task_id"`
	StartedAt  time.Time    `json:"started_at"`
	EndedAt    time.Time    `json:"ended_at"`
	DurationMS int64        `json:"duration_ms"`
	Tokens     *TokenFigure `json:"tokens"`
}

// TokenSection is a run's token use as a whole. When Status is unknown,
// ReasonCode and Reason say why, the figures are null and the lists empty.
// The step figures plus OutsideSteps equal Total for each token type;
// SessionOverhead, the tokens Claude Code counted that its transcripts
// never record, is part of OutsideSteps.
type TokenSection struct {
	Status                string            `json:"status"`
	ReasonCode            *string           `json:"reason_code"`
	Reason                *string           `json:"reason"`
	Agent                 string            `json:"agent"`
	Sessions              []SessionRef      `json:"sessions"`
	SkippedLines          int               `json:"skipped_lines"`
	Total                 *TokenFigure      `json:"total"`
	OutsideSteps          *TokenFigure      `json:"outside_steps"`
	SessionOverhead       *TokenFigure      `json:"session_overhead"`
	SessionOverheadReason *string           `json:"session_overhead_reason"`
	Models                []ModelTokens     `json:"models"`
	UnmatchedHelpers      []UnmatchedHelper `json:"unmatched_helpers"`
}

// tokenRows are the token figures of a report's rows: per step name, per
// task key (its task id, or "span:" and its span id when it has none) and
// per helper span id. A row with no entry has unknown token use.
type tokenRows struct {
	steps   map[string]*TokenFigure
	tasks   map[string]*TokenFigure
	helpers map[string]*TokenFigure
}

// usageByModel is token use split by model.
type usageByModel map[string]agentusage.Usage

func (u usageByModel) add(model string, v agentusage.Usage) {
	u[model] = u[model].Add(v)
}

func (u usageByModel) addAll(v usageByModel) {
	for m, x := range v {
		u.add(m, x)
	}
}

func (u usageByModel) sum() agentusage.Usage {
	var s agentusage.Usage
	for _, x := range u {
		s = s.Add(x)
	}
	return s
}
