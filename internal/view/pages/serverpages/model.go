package serverpages

import "github.com/irgordon/gopanel/internal/view/pages/containerpages"

type DisplayServer struct {
	ID             string
	Name           string
	Address        string
	ConnectionType string
}

type DisplayInput struct {
	Name           string
	Address        string
	ConnectionType string
}

type DetailModel struct {
	Server DisplayServer
	Docker *containerpages.DockerSummaryModel
}
