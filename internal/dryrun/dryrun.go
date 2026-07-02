package dryrun

import (
	"fmt"
	"strings"
)

type Result struct {
	DryRun           bool           `json:"dry_run"`
	Command          string         `json:"command"`
	Operation        string         `json:"operation"`
	WouldSendRequest bool           `json:"would_send_request"`
	Request          RequestSummary `json:"request"`
	Checks           []Check        `json:"checks,omitempty"`
}

type RequestSummary struct {
	Description string `json:"description,omitempty"`
	Method      string `json:"method,omitempty"`
	Path        string `json:"path,omitempty"`
	Body        any    `json:"body,omitempty"`
}

type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func New(command, operation string, request RequestSummary, checks ...Check) Result {
	allChecks := []Check{
		{
			Name:   "input_validation",
			Status: "passed",
		},
	}
	allChecks = append(allChecks, checks...)
	allChecks = append(allChecks, Check{
		Name:    "remote_mutation",
		Status:  "skipped",
		Message: "dry-run stopped before sending a mutating remote request",
	})

	return Result{
		DryRun:           true,
		Command:          command,
		Operation:        operation,
		WouldSendRequest: true,
		Request:          request,
		Checks:           allChecks,
	}
}

func (r Result) Human() string {
	lines := []string{
		"Dry run: " + r.Command,
		"Operation: " + r.Operation,
		fmt.Sprintf("Would send request: %t", r.WouldSendRequest),
	}
	if r.Request.Method != "" || r.Request.Path != "" {
		lines = append(lines, "Request: "+strings.TrimSpace(r.Request.Method+" "+r.Request.Path))
	}
	if r.Request.Description != "" {
		lines = append(lines, "Request: "+r.Request.Description)
	}
	for _, check := range r.Checks {
		if check.Message == "" {
			lines = append(lines, fmt.Sprintf("Check %s: %s", check.Name, check.Status))
			continue
		}
		lines = append(lines, fmt.Sprintf("Check %s: %s - %s", check.Name, check.Status, check.Message))
	}
	return strings.Join(lines, "\n")
}
