package main

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"

	"langswitch/internal/update"
)

// version задаётся при сборке: -ldflags "-X main.version=v1.2.3" (см. Makefile).
var version = "dev"

const (
	updateDelay    = 10 * time.Second // первая проверка — после запуска, чтобы не мешать ему
	updateInterval = 24 * time.Hour
	updateTimeout  = 15 * time.Second
)

// updater периодически проверяет, не вышла ли новая версия.
type updater struct {
	enabled atomic.Bool
	found   func(update.Release) // вызывается в UI-потоке, когда найдена новая версия
}

func (u *updater) run() {
	t := time.NewTimer(updateDelay)
	for range t.C {
		if u.enabled.Load() {
			rel, newer, err := u.check()
			switch {
			case err != nil:
				log.Printf("обновления: %v", err)
			case newer:
				fyne.Do(func() { u.found(rel) })
			}
		}
		t.Reset(updateInterval)
	}
}

// check запрашивает последний релиз и сообщает, новее ли он текущей версии.
func (u *updater) check() (update.Release, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
	defer cancel()
	rel, err := update.Latest(ctx)
	if err != nil {
		return update.Release{}, false, err
	}
	return rel, update.Newer(rel.Version, version), nil
}
