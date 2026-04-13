package models

type User struct {
	ID             string
	Name           string
	EmployeeID     string
	CorporateEmail string
	Role           string
	Department     string
	Position       string
	PasswordTemp   bool
}
