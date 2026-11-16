package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/norarrsgd/gosmos"
)

func syntheticProduct(partition, name string) product {
	return product{Base: gosmos.Base{ID: "gosmos-example-" + rand.Text()}, TenantID: partition, Name: name, Description: "Synthetic example document", Category: "Examples", Price: 1, Quantity: 1}
}

// Cleanup gets its own bounded context because the demo's request may have timed out.
func removeCreated(galaxy *gosmos.Galaxy[product], items []product) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var failures []error
	for _, item := range items {
		_, e := galaxy.Remove(ctx, item.ID, []string{item.TenantID})
		if e != nil && !errors.Is(e, gosmos.ErrNotFound) {
			failures = append(failures, fmt.Errorf("cleanup %s: %w", item.ID, e))
		}
	}
	return errors.Join(failures...)
}
func singleItem(ctx context.Context, out io.Writer, galaxy *gosmos.Galaxy[product], partition string) (err error) {
	item := syntheticProduct(partition, "Gosmos example")
	fmt.Fprintf(out, "Synthetic document ID: %s\n", item.ID)
	created, gravity, err := galaxy.Create(ctx, item)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, removeCreated(galaxy, []product{created})) }()
	first, _, err := galaxy.Get(ctx, created.ID, []string{partition})
	if err != nil {
		return err
	}
	_, cached, err := galaxy.Get(ctx, created.ID, []string{partition})
	if err != nil {
		return err
	}
	first.Description = "Updated with optimistic concurrency"
	if _, _, err = galaxy.Modify(ctx, first, gosmos.IfMatch(gravity.ETag)); err != nil {
		return err
	}
	fmt.Fprintf(out, "Created, read twice, and replaced; second read RU: %.2f; cleanup follows.\n", cached.RU)
	return nil
}
func bulkItems(ctx context.Context, out io.Writer, galaxy *gosmos.Galaxy[product], partition string) (err error) {
	batch := galaxy.Bulk()
	for i := range 3 {
		item := syntheticProduct(partition, fmt.Sprintf("Gosmos bulk %d", i+1))
		fmt.Fprintf(out, "Synthetic document ID: %s\n", item.ID)
		batch.Create(item)
	}
	result, executeErr := batch.Execute(ctx)
	var committed []product
	for _, item := range result.Items {
		if item.Err == nil && item.Status >= 200 && item.Status < 300 {
			committed = append(committed, item.Item)
		}
	}
	defer func() { err = errors.Join(err, removeCreated(galaxy, committed)) }()
	fmt.Fprintf(out, "Confirmed creations: %d; RU: %.2f; cleanup follows.\n", len(committed), result.Gravity.RU)
	return executeErr
}
func atomicItem(ctx context.Context, out io.Writer, galaxy *gosmos.Galaxy[product], partition string) (err error) {
	item := syntheticProduct(partition, "Gosmos atomic example")
	fmt.Fprintf(out, "Synthetic document ID: %s\n", item.ID)
	created, gravity, err := galaxy.Create(ctx, item)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, removeCreated(galaxy, []product{created})) }()
	created.Description = "Atomically replaced"
	replaced, err := galaxy.Atomic(partition).Replace(created, gosmos.IfMatch(gravity.ETag)).Execute(ctx)
	if err != nil {
		return err
	}
	if len(replaced.Items) != 1 || replaced.Items[0].ETag == "" {
		return errors.New("atomic replace did not return an ETag")
	}
	patched, err := galaxy.Atomic(partition).Patch(created.ID, func(p *gosmos.Patch[product]) { p.Increment("quantity", 1) }, gosmos.IfMatch(replaced.Items[0].ETag)).Execute(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "ETag-protected replace and patch completed; RU: %.2f; cleanup follows.\n", gravity.RU+replaced.Gravity.RU+patched.Gravity.RU)
	return nil
}
