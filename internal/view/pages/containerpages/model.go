package containerpages

type DisplayContainer struct {
	ID      string
	ShortID string
	Name    string
	Image   string
	State   string
	Status  string
}

type DockerSummaryModel struct {
	ServerID       string
	State          string
	StatusLabel    string
	Freshness      string
	ErrorReference string
	CSRFToken      string
	SuccessMessage string
}

type ContainerListModel struct {
	ServerID    string
	ServerName  string
	Containers  []DisplayContainer
	CanViewLogs bool
}

type ErrorModel struct {
	ServerID       string
	Title          string
	Message        string
	ErrorReference string
	ShowErrorLog   bool
}

type LogsModel struct {
	ServerID     string
	ContainerID  string
	ContainerRef string
	Content      string
}
