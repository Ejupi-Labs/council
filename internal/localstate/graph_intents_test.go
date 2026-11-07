package localstate

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func graphIntentFixture(t *testing.T) *Lock {
	t.Helper()
	lock, err := Acquire(filepath.Join(t.TempDir(), "v2"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	return lock
}

func TestGraphIntentPrivatePublicationIsImmutableAcrossReopen(t *testing.T) {
	owner := graphIntentFixture(t)
	id := strings.Repeat("1", 32)
	intent, err := OpenGraphIntent(context.Background(), owner.Root(), id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = intent.Close() })
	if body, seen, err := intent.Read(); err != nil || seen || body != nil {
		t.Fatal("fresh identity invented retained bytes", seen, err)
	}
	if err = intent.MarkSeen(); err != nil {
		t.Fatal(err)
	}
	body := []byte("opaque private payload\r\n東京\x00\xff")
	if err = intent.Write(body); err != nil {
		t.Fatal(err)
	}
	if err = intent.Write([]byte("replacement")); err == nil {
		t.Fatal("immutable publication was overwritten")
	}
	actual, seen, err := intent.Read()
	if err != nil || !seen || !bytes.Equal(body, actual) {
		t.Fatal("known body was not retained exactly", seen, err)
	}
	if err = verifyPrivate(filepath.Join(owner.Root(), "client-intents", "graphs", id, "body.json"), false); err != nil {
		t.Fatal("published body is not an ordinary private single-link file", err)
	}
	if err = intent.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenGraphIntent(context.Background(), owner.Root(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	actual, seen, err = reopened.Read()
	if err != nil || !seen || !bytes.Equal(body, actual) {
		t.Fatal("reopen lost the immutable body", seen, err)
	}
	if other, err := Acquire(owner.Root()); !errors.Is(err, ErrLocked) {
		if other != nil {
			_ = other.Close()
		}
		t.Fatal("request lock changed the continuing owner lock", err)
	}
}

func TestGraphIntentNativeLockSerializesOnlyTheSameRequest(t *testing.T) {
	owner := graphIntentFixture(t)
	id := strings.Repeat("2", 32)
	first, err := OpenGraphIntent(context.Background(), owner.Root(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	other, err := OpenGraphIntent(context.Background(), owner.Root(), strings.Repeat("3", 32))
	if err != nil {
		t.Fatal("independent request was blocked", err)
	}
	defer other.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if duplicate, err := OpenGraphIntent(ctx, owner.Root(), id); !errors.Is(err, context.DeadlineExceeded) {
		if duplicate != nil {
			_ = duplicate.Close()
		}
		t.Fatal("same request escaped its native lock", err)
	}
	stillBlocked, end := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer end()
	if duplicate, err := OpenGraphIntent(stillBlocked, owner.Root(), id); !errors.Is(err, context.DeadlineExceeded) {
		if duplicate != nil {
			_ = duplicate.Close()
		}
		t.Fatal("cancelled contender released another handle's native lock", err)
	}
	if err := first.MarkSeen(); err != nil {
		t.Fatal("cancelled contender changed the retained lock holder", err)
	}
	if err := first.Write([]byte("still owned")); err != nil {
		t.Fatal(err)
	}
}

func TestGraphIntentSeenWithoutBodyRemainsUnresolvedAcrossReopen(t *testing.T) {
	owner := graphIntentFixture(t)
	id := strings.Repeat("4", 32)
	first, err := OpenGraphIntent(context.Background(), owner.Root(), id)
	if err != nil {
		t.Fatal(err)
	}
	if err = first.MarkSeen(); err != nil {
		t.Fatal(err)
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	retry, err := OpenGraphIntent(context.Background(), owner.Root(), id)
	if err != nil {
		t.Fatal(err)
	}
	defer retry.Close()
	if body, seen, err := retry.Read(); !errors.Is(err, ErrGraphIntentUnresolved) || !seen || body != nil {
		t.Fatal("an incomplete durable marker became a fresh identity", seen, err)
	}
}
