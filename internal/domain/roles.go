package domain

type Role string

const (
	RoleManager   Role = "manager"
	RoleAnnotator Role = "annotator"
	RoleExpert    Role = "expert"
	RoleLead      Role = "lead"
)

func ParseRole(value string) (Role, error) {
	r := Role(value)
	switch r {
	case RoleManager, RoleAnnotator, RoleExpert, RoleLead:
		return r, nil
	default:
		return "", NewError(CodeForbidden, "角色 %q 无权执行该操作", value)
	}
}

func RequireRole(actual Role, allowed ...Role) error {
	for _, role := range allowed {
		if actual == role {
			return nil
		}
	}
	return NewError(CodeForbidden, "角色 %q 无权执行该操作", actual)
}
