package controllers

type Port struct {
	Name   string
	Number int32
}

type NodeTaskStatus int

const (
	NodeTaskStatusUnprocessed NodeTaskStatus = 1
	NodeTaskStatusAssigned                   = 2
	NodeTaskStatusProcessing                 = 3
	NodeTaskStatusComplete                   = 10
	NodeTaskStatusFailed                     = 22
	NodeTaskStatusDeleted                    = 99
)
