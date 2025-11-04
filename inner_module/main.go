package inner_module

import (
	"context"
	"database/sql"
	"github.com/heroiclabs/nakama-common/runtime"
)

var (
	errNoUserIdFound = runtime.NewError("no user ID in context", 1) // INVALID_ARGUMENT
)

func InitModule(ctx context.Context, logger runtime.Logger, db *sql.DB, nk runtime.NakamaModule, initializer runtime.Initializer) error {
	if err := initializer.RegisterAfterAuthenticateDevice(InitializeDeviceUser); err != nil {
		logger.Error("Unable to register: %v", err)
		return err
	}

	if err := initializer.RegisterAfterAuthenticateCustom(InitializeCustomUser); err != nil {
		logger.Error("Unable to register: %v", err)
		return err
	}

	if err := initializer.RegisterAfterGetAccount(InitializeAccount); err != nil {
		logger.Error("Unable to register: %v", err)
		return err
	}
	logger.Info("Inner module initialized with challenge reward callbacks")
	return nil
}
