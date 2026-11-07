package clash

import (
	"context"
	"errors"

	"github.com/mhsanaei/3x-ui/v3/internal/clash/allocation"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/passkey"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/policy"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/subscription"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
)

func Migrate() error {
	db := database.GetDB()
	if db == nil {
		return errors.New("clash: database is not initialized")
	}
	if err := db.AutoMigrate(
		&subscription.RuleSet{},
		&subscription.YAMLTemplate{},
		&subscription.SubscriptionBinding{},
		&allocation.NodeWeight{},
		&allocation.ClientRatio{},
		&policy.Delivery{},
		&policy.AppliedPolicy{},
	); err != nil {
		return err
	}
	if err := passkey.New(db).Migrate(); err != nil {
		return err
	}
	return subscription.MigratePublications(db)
}

func Start(ctx context.Context) error {
	rt := policy.GetRuntime()
	if rt == nil {
		return errors.New("clash runtime is not configured")
	}
	return rt.Start(ctx)
}
