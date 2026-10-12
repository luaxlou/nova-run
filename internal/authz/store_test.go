package authz

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetOverwritesSingleProjectAuthorization(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	first, err := store.Set("billing")
	if err != nil {
		t.Fatal(err)
	}
	if !store.Verify("billing", first) || store.Verify("orders", first) {
		t.Fatal("first token has incorrect project scope")
	}
	second, err := store.Set("billing")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("replacement token did not change")
	}
	if store.Verify("billing", first) || !store.Verify("billing", second) {
		t.Fatal("replacement did not invalidate old token")
	}
	info, err := os.Stat(filepath.Join(store.Dir, "billing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("record mode=%o want=600", info.Mode().Perm())
	}
}

func TestProjectAuthorizationIsIndependent(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	billing, _ := store.Set("billing")
	orders, _ := store.Set("orders")
	if store.Verify("orders", billing) || store.Verify("billing", orders) {
		t.Fatal("cross-project token unexpectedly verified")
	}
}

func TestValidateProjectNameRejectsAmbiguousOrUnsafeNames(t *testing.T) {
	for _, name := range []string{"", "Billing", "../billing", "billing/api", "billing--api", "-billing", "billing-"} {
		if ValidateProjectName(name) == nil {
			t.Fatalf("project name %q unexpectedly accepted", name)
		}
	}
}
