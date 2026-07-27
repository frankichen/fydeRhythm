package model

import "encoding/json"

const Version = "0.2.1"

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

type Change struct {
	ChangeID        string          `json:"change_id"`
	Entity          string          `json:"entity"`
	Operation       string          `json:"operation"`
	ClientUpdatedAt int64           `json:"client_updated_at"`
	Payload         json.RawMessage `json:"payload"`
}

type Device struct {
	DeviceID   string `json:"device_id"`
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	AppVersion string `json:"app_version"`
	LastSeenAt int64  `json:"last_seen_at,omitempty"`
}

type PushRequest struct {
	DeviceID string   `json:"device_id"`
	Device   *Device  `json:"device,omitempty"`
	Changes  []Change `json:"changes"`
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
	Rejected   []RejectedChange `json:"rejected"`
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

type Snapshot struct {
	Cursor      int64          `json:"cursor"`
	GeneratedAt int64          `json:"generated_at"`
	Lexicon     []LexiconEntry `json:"lexicon"`
}
