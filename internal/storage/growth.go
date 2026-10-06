package storage

type GrowthStep struct {
	Title       string `json:"title"`
	Command     string `json:"command,omitempty"`
	Explanation string `json:"explanation"`
}
type GrowthPlan struct {
	ID              string       `json:"id"`
	VolumeID        string       `json:"volume_id"`
	Mount           string       `json:"mount"`
	Source          string       `json:"source"`
	FSType          string       `json:"fs_type"`
	Environment     string       `json:"environment"`
	DeviceBytes     int64        `json:"device_bytes"`
	FilesystemBytes int64        `json:"filesystem_bytes"`
	CanGrow         bool         `json:"can_grow"`
	Message         string       `json:"message"`
	Steps           []GrowthStep `json:"steps"`
}
type GrowthState struct {
	Status      string `json:"status"`
	StartedAt   string `json:"started_at,omitempty"`
	FinishedAt  string `json:"finished_at,omitempty"`
	VolumeID    string `json:"volume_id,omitempty"`
	Mount       string `json:"mount,omitempty"`
	BeforeBytes int64  `json:"before_bytes"`
	AfterBytes  int64  `json:"after_bytes"`
	Message     string `json:"message,omitempty"`
}

func ShellQuote(value string) string {
	out := "'"
	for _, r := range value {
		if r == '\'' {
			out += "'\\''"
		} else {
			out += string(r)
		}
	}
	return out + "'"
}
