package http

import "xxxdonacion/modules/identity/api/domain"

type userResponse struct {
	ID     string        `json:"id"`
	Email  string        `json:"email"`
	Name   string        `json:"name,omitempty"`
	Status domain.Status `json:"status"`
	Roles  []domain.Role `json:"roles"`
}

func newUserResponse(u *domain.User) userResponse {
	return userResponse{
		ID:     u.ID.String(),
		Email:  u.Email,
		Name:   u.Name,
		Status: u.Status,
		Roles:  u.Roles,
	}
}

// meResponse adalah representasi identitas caller pada endpoint /me.
type meResponse struct {
	UID           string `json:"uid"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	EmailVerified bool   `json:"email_verified"`
}
