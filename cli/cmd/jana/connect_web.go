package main

import (
	"context"
	"example.com/jana-locate-cli/internal/app"
	"fmt"
	"os"
	"time"
)

func connectWeb(ctx context.Context, o flowOptions, saved profile) error {
	showConnection(saved)
	store, err := app.Open(o.state)
	if err != nil {
		return err
	}
	defer store.DB.Close()
	token, err := secret("JANA_API_TOKEN")
	if err != nil {
		return err
	}
	n, err := app.NewNormalizer(o.apiURL, token)
	if err != nil {
		return err
	}
	web, err := startWeb(ctx, store, n, app.Postgres{DSN: saved.DatabaseURL, APIURL: o.apiURL}, o.state, o.listen, o.profilePath)
	if err != nil {
		return err
	}
	defer web.Close()
	openSite(web.URL + "?connect=true")
	fmt.Fprintln(os.Stderr, "Ожидаем сохранения подключения в браузере. Ctrl+C — отмена.")
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			p, err := readProfile(o.profilePath)
			if err == nil && p.VerifiedAt != saved.VerifiedAt {
				fmt.Fprintln(os.Stderr, "✓ Подключение проверено и сохранено. Далее: jana normalize")
				return nil
			}
		}
	}
}
