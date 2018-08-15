package main

type Payload struct {
	ID      string `json:"id,omitempty"`
	Data    string `json:"data,omitempty"`
	Slug    string `json:"slug,omitempty"`
	Message string `json:"message,omitempty"`
	Success bool   `json:"success,omitempty"`
}
