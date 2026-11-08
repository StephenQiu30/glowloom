package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	scopewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/scope"
	billinghttp "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/http"
	billingpg "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/postgres"
	billingredis "github.com/StephenQiu30/glowloom-server/backend/internal/billing/adapter/redis"
	billingapp "github.com/StephenQiu30/glowloom-server/backend/internal/billing/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func walletReader(db *gorm.DB, redisClient *redis.Client) billinghttp.WalletReader {
	cache := billingredis.NewWalletBalanceCache(redisClient)
	return func(ctx context.Context, actor identityapp.Principal, cursor *uuid.UUID) (billingapp.WalletView, error) {
		var view billingapp.WalletView
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			scope, err := scopewire.ProvideRequestScopeAccess(tx).Resolve(ctx, actor, actor.ScopeID, false)
			if errors.Is(err, workspaceapp.ErrScopeNotFound) {
				return identityapp.ErrForbidden
			}
			if err != nil {
				return err
			}
			history := scope.Kind == "personal"
			if scope.TeamID != nil {
				err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM workspace.team_member WHERE team_id=? AND user_id=?
					AND role='team_admin' AND status='active')`, *scope.TeamID, actor.ID).Row().Scan(&history)
				if err != nil {
					return err
				}
			}
			var cached *billingapp.WalletBalance
			if balance, ok, cacheErr := cache.Get(ctx, scope.ID); cacheErr == nil && ok {
				cached = &balance
			}
			var balance billingapp.WalletBalance
			view, balance, err = billingpg.NewStore(tx).ReadWalletWithBalance(ctx, scope.ID, history, cursor, cached)
			if err == nil && cached == nil {
				_ = cache.Put(ctx, scope.ID, balance)
			}
			view.Payer = scope.Kind
			return err
		}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
		return view, err
	}
}
