package domain

import (
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"

	"xxxdonacion/platform/apperr"
)

type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusDisabled Status = "DISABLED"
)

func (s Status) Valid() bool {
	return s == StatusActive || s == StatusDisabled
}

// Principal adalah identitas pemanggil yang sudah diverifikasi Firebase.
// Role TIDAK diambil dari token; role selalu dibaca dari database.
type Principal struct {
	UID           string
	Email         string
	Name          string
	EmailVerified bool
}

func (p Principal) Validate() error {
	if p.UID == "" {
		return apperr.Unauthorized("unauthenticated")
	}
	if p.Email == "" || !p.EmailVerified {
		return apperr.Forbidden("email must be verified")
	}
	return nil
}

type User struct {
	ID        uuid.UUID `json:"id"`
	AuthID    string    `json:"auth_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name,omitempty"`
	Status    Status    `json:"status"`
	Roles     []Role    `json:"roles"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateUserInput struct {
	Email    string
	Name     string
	Password string
}

// Validate memvalidasi sekaligus menormalkan input.
func (in *CreateUserInput) Validate() error {
	in.Email = strings.TrimSpace(in.Email)
	addr, err := mail.ParseAddress(in.Email)
	if err != nil || !strings.EqualFold(addr.Address, in.Email) {
		return apperr.BadRequest("invalid email")
	}
	in.Email = strings.ToLower(in.Email)

	in.Name = strings.TrimSpace(in.Name)
	if utf8.RuneCountInString(in.Name) > 100 {
		return apperr.BadRequest("name must be at most 100 characters")
	}

	if in.Password != "" && len(in.Password) < 12 {
		return apperr.BadRequest("password must be at least 12 characters")
	}
	return nil
}

type UpdateStatusInput struct {
	Status Status
}

func (in *UpdateStatusInput) Validate() error {
	in.Status = Status(strings.ToUpper(strings.TrimSpace(string(in.Status))))
	if !in.Status.Valid() {
		return apperr.BadRequest("status must be ACTIVE or DISABLED")
	}
	return nil
}
