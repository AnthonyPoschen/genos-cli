package cli

import (
	"context"
	"encoding/json"
	"strconv"
	"time"
)

func (r *runner) me(asJSON bool) int {
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	raw, err := client.GetMeRaw(ctx)
	if err != nil {
		return r.fail(err)
	}
	if asJSON {
		return r.printRawJSON(raw)
	}
	var me struct {
		ID              string `json:"id"`
		ClerkUserID     string `json:"clerkUserID"`
		SessionID       string `json:"sessionID"`
		PrimaryEmail    string `json:"primaryEmail"`
		IsPlatformAdmin bool   `json:"isPlatformAdmin"`
	}
	if err := json.Unmarshal(raw, &me); err != nil {
		return r.fail(err)
	}
	WriteTable(r.out, []string{"ID", "CLERK USER", "SESSION", "EMAIL", "ADMIN"}, [][]string{{
		me.ID,
		me.ClerkUserID,
		me.SessionID,
		me.PrimaryEmail,
		strconv.FormatBool(me.IsPlatformAdmin),
	}})
	return 0
}

func (r *runner) dashboard(args []string) int {
	if len(args) != 0 {
		return r.usage("dashboard takes no arguments")
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	raw, err := client.GetDashboardRaw(ctx)
	if err != nil {
		return r.fail(err)
	}
	return r.printRawJSON(raw)
}

func (r *runner) catalog(asJSON bool) int {
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	raw, err := client.GetCatalogRaw(ctx)
	if err != nil {
		return r.fail(err)
	}
	if asJSON {
		return r.printRawJSON(raw)
	}
	var catalog struct {
		Games []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"games"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return r.fail(err)
	}
	if len(catalog.Games) == 0 {
		WriteEmpty(r.out, "No games.")
		return 0
	}
	rows := make([][]string, 0, len(catalog.Games))
	for _, game := range catalog.Games {
		rows = append(rows, []string{game.ID, game.Name})
	}
	WriteTable(r.out, []string{"ID", "NAME"}, rows)
	return 0
}
