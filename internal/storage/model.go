package storage

type Report struct {
	CollectedAt string   `json:"collected_at"`
	DataPath    string   `json:"data_path"`
	Environment string   `json:"environment"`
	Volumes     []Volume `json:"volumes"`
	Devices     []Device `json:"devices"`
	Warnings    []string `json:"warnings"`
	Demo        bool     `json:"demo"`
}
type Volume struct {
	ID          string   `json:"id"`
	Mount       string   `json:"mount"`
	Paths       []string `json:"paths"`
	Source      string   `json:"source"`
	FSType      string   `json:"fs_type"`
	UUID        string   `json:"uuid,omitempty"`
	Total       int64    `json:"total"`
	Used        int64    `json:"used"`
	Available   int64    `json:"available"`
	Reserved    int64    `json:"reserved"`
	UsedPercent float64  `json:"used_percent"`
	Inodes      int64    `json:"inodes"`
	InodesUsed  int64    `json:"inodes_used"`
	ReadOnly    bool     `json:"read_only"`
	IsData      bool     `json:"is_data"`
	IsSystem    bool     `json:"is_system"`
	Error       string   `json:"error,omitempty"`
	History     []Sample `json:"history,omitempty"`
	Forecast    Forecast `json:"forecast"`
}
type Device struct {
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	Size     int64     `json:"size"`
	FSType   string    `json:"fstype"`
	UUID     string    `json:"uuid"`
	Mounts   []*string `json:"mountpoints"`
	Parent   string    `json:"pkname"`
	Children []Device  `json:"children,omitempty"`
}
type Sample struct {
	At        string `json:"at"`
	Total     int64  `json:"total"`
	Used      int64  `json:"used"`
	Available int64  `json:"available"`
}
type Forecast struct {
	Status       string `json:"status"`
	Message      string `json:"message"`
	GrowthPerDay int64  `json:"growth_per_day"`
	DaysToFull   *int   `json:"days_to_full,omitempty"`
	FullAt       string `json:"full_at,omitempty"`
	BasedOnDays  int    `json:"based_on_days"`
}
type Stats struct{ Total, Free, Available, Inodes, InodesFree int64 }
