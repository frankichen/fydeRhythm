package store

import "encoding/json"

type Change struct {
	ChangeID        string          `json:"change_id"`
	Entity          string          `json:"entity"`
	Operation       string          `json:"operation"`
	ClientUpdatedAt int64           `json:"client_updated_at"`
	Payload         json.RawMessage `json:"payload"`
}

type PushRequest struct {
	DeviceID string   `json:"device_id"`
	Device   *Device  `json:"device,omitempty"`
	Changes  []Change `json:"changes"`
}

type Device struct {
	DeviceID   string `json:"device_id"`
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	AppVersion string `json:"app_version"`
	LastSeenAt int64  `json:"last_seen_at"`
}

type RejectedChange struct {
	ChangeID string `json:"change_id"`
	Reason   string `json:"reason"`
}

type PushResult struct {
	Cursor     int64            `json:"cursor"`
	Applied    int              `json:"applied"`
	Duplicates int              `json:"duplicates"`
	Conflicts  int              `json:"conflicts"`
	Rejected   []RejectedChange `json:"rejected,omitempty"`
}

type LogChange struct {
	Seq            int64           `json:"seq"`
	ChangeID       string          `json:"change_id"`
	Entity         string          `json:"entity"`
	Operation      string          `json:"operation"`
	EntityKey      string          `json:"entity_key"`
	Payload        json.RawMessage `json:"payload"`
	SourceDeviceID string          `json:"source_device_id"`
	CreatedAt      int64           `json:"created_at"`
}

type PullResult struct {
	Cursor  int64       `json:"cursor"`
	HasMore bool        `json:"has_more"`
	Changes []LogChange `json:"changes"`
}

type LexiconEntry struct {
	ID              string `json:"id"`
	Phrase          string `json:"phrase"`
	Code            string `json:"code"`
	Shortcut        string `json:"shortcut"`
	Weight          int    `json:"weight"`
	Category        string `json:"category"`
	Notes           string `json:"notes"`
	Deleted         bool   `json:"deleted"`
	ClientUpdatedAt int64  `json:"client_updated_at"`
	SourceDeviceID  string `json:"source_device_id"`
	ServerUpdatedAt int64  `json:"server_updated_at"`
	Version         int64  `json:"version"`
}

type CandidateUsage struct {
	Code       string `json:"code"`
	Candidate  string `json:"candidate"`
	Count      int64  `json:"count"`
	LastUsedAt int64  `json:"last_used_at"`
}

type Correction struct {
	Original   string `json:"original"`
	Corrected  string `json:"corrected"`
	Count      int64  `json:"count"`
	LastUsedAt int64  `json:"last_used_at"`
}

type AIFeedback struct {
	Feature       string `json:"feature"`
	ContextHash   string `json:"context_hash"`
	AcceptedCount int64  `json:"accepted_count"`
	RejectedCount int64  `json:"rejected_count"`
	LastUsedAt    int64  `json:"last_used_at"`
}

type Setting struct {
	Key             string          `json:"key"`
	Value           json.RawMessage `json:"value"`
	Deleted         bool            `json:"deleted"`
	ClientUpdatedAt int64           `json:"client_updated_at"`
	SourceDeviceID  string          `json:"source_device_id"`
	ServerUpdatedAt int64           `json:"server_updated_at"`
	Version         int64           `json:"version"`
}

type Snapshot struct {
	Cursor         int64            `json:"cursor"`
	GeneratedAt    int64            `json:"generated_at"`
	Lexicon        []LexiconEntry   `json:"lexicon"`
	CandidateUsage []CandidateUsage `json:"candidate_usage"`
	Corrections    []Correction     `json:"corrections"`
	AIFeedback     []AIFeedback     `json:"ai_feedback"`
	Settings       []Setting        `json:"settings"`
}
