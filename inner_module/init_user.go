package inner_module

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/heroiclabs/nakama-common/api"
	"github.com/heroiclabs/nakama-common/runtime"
)

func InitWallet(ctx context.Context, logger runtime.Logger, nk runtime.NakamaModule) error {

	userID, ok := ctx.Value(runtime.RUNTIME_CTX_USER_ID).(string)
	if !ok {
		return errNoUserIdFound
	}
	//初始化钱包
	initWallet := map[string]int64{
		"coin": 0,
		"gem":  0,
		"ad":   0,
	}
	if updated, previous, err := nk.WalletUpdate(ctx, userID, initWallet, nil, false); err != nil {
		for key, value := range updated {
			logger.Error("update wallet error updated Key:", key, "Value:", value)
		}
		for key, value := range previous {
			logger.Error("update wallet error previous Key:", key, "Value:", value)
		}
		return err
	}

	return nil
}

func InitializeDeviceUser(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, out *api.Session, in *api.AuthenticateDeviceRequest) error {
	if out.Created {
		return InitWallet(ctx, logger, nk)
	}
	return nil
}

func InitializeCustomUser(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, out *api.Session, in *api.AuthenticateCustomRequest) error {
	if out.Created {
		return InitWallet(ctx, logger, nk)
	}
	return nil
}

func InitializeAccount(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, out *api.Account) error {
	var wallet map[string]int64
	err := json.Unmarshal([]byte(out.Wallet), &wallet)
	if err != nil {
		return err
	}
	if len(wallet) == 0 {
		return InitWallet(ctx, logger, nk)
	}
	return nil
}
