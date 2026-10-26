package req

import "xxxdonacion/modules/identity/api/domain"

type CreateUser struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

func (r CreateUser) Input() domain.CreateUserInput {
	return domain.CreateUserInput{
		Email:    r.Email,
		Name:     r.Name,
		Password: r.Password,
	}
}

type UpdateStatus struct {
	Status string `json:"status"`
}

func (r UpdateStatus) Input() domain.UpdateStatusInput {
	return domain.UpdateStatusInput{Status: domain.Status(r.Status)}
}

type UpdateRoles struct {
	Roles []string `json:"roles"`
}

func (r UpdateRoles) Input() domain.UpdateRolesInput {
	return domain.UpdateRolesInput{Roles: toRoles(r.Roles)}
}

func toRoles(roles []string) []domain.Role {
	if roles == nil {
		return nil
	}
	out := make([]domain.Role, 0, len(roles))
	for _, role := range roles {
		out = append(out, domain.Role(role))
	}
	return out
}
