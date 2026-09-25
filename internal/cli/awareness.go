package cli

import (
	"context"
	"time"
)

func (r *runner) me(args []string) int {
	if len(args) != 0 {
		return r.usage("me takes no arguments")
	}
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
	return r.printRawJSON(raw)
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

func (r *runner) catalog(args []string) int {
	if len(args) != 0 {
		return r.usage("catalog takes no arguments")
	}
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
	return r.printRawJSON(raw)
}
