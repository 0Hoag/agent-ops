package insight

import "context"

type contextKey string

const organizationKey contextKey = "organization_id"

func organizationFromContext(ctx context.Context) string {
	v, _ := ctx.Value(organizationKey).(string)
	return v
}
func WithOrganization(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, organizationKey, id)
}
