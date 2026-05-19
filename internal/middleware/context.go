package middleware

import (
	"context"

	"rehab-app/internal/models"
)

type contextKey string

const userKey contextKey = "user"

func WithUser(ctx context.Context, user *models.User) context.Context {
	return context.WithValue(ctx, userKey, user)
}

func UserFromContext(ctx context.Context) *models.User {
	user, _ := ctx.Value(userKey).(*models.User)
	return user
}
