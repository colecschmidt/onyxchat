package store_test

import (
	"errors"
	"testing"

	"github.com/cole/onyxchat-server/internal/store"
)

// enroll puts a user through the full pending → confirmed transition and
// returns the recovery code hashes that were installed.
func enroll(t *testing.T, us *store.UserStore, userID int64, secret string) []string {
	t.Helper()
	if err := us.SetPendingTOTPSecret(userID, secret); err != nil {
		t.Fatalf("SetPendingTOTPSecret: %v", err)
	}
	hashes := []string{"hash-a-" + uid(), "hash-b-" + uid(), "hash-c-" + uid()}
	if err := us.ConfirmTOTP(userID, hashes); err != nil {
		t.Fatalf("ConfirmTOTP: %v", err)
	}
	return hashes
}

func TestGetTOTPEnrollment_NotEnrolled(t *testing.T) {
	us := store.NewUserStore(testDB)
	u := seedUser(t, "u"+uid())

	_, err := us.GetTOTPEnrollment(u.ID)
	if !errors.Is(err, store.ErrTOTPNotEnrolled) {
		t.Fatalf("err = %v, want ErrTOTPNotEnrolled", err)
	}
}

func TestSetPendingTOTPSecret_StaysDisabledUntilConfirmed(t *testing.T) {
	us := store.NewUserStore(testDB)
	u := seedUser(t, "u"+uid())

	if err := us.SetPendingTOTPSecret(u.ID, "encrypted-secret"); err != nil {
		t.Fatalf("SetPendingTOTPSecret: %v", err)
	}

	enr, err := us.GetTOTPEnrollment(u.ID)
	if err != nil {
		t.Fatalf("GetTOTPEnrollment: %v", err)
	}
	if enr.Secret != "encrypted-secret" {
		t.Fatalf("secret = %q", enr.Secret)
	}
	if enr.Enabled {
		t.Fatal("a pending enrollment reports as enabled")
	}

	// A pending enrollment must not show up on the user record, which is what
	// the login path reads.
	reloaded, err := us.GetUserByID(u.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if reloaded.TOTPEnabled {
		t.Fatal("pending enrollment gates login")
	}
}

func TestConfirmTOTP_EnablesAndInstallsCodes(t *testing.T) {
	us := store.NewUserStore(testDB)
	u := seedUser(t, "u"+uid())
	hashes := enroll(t, us, u.ID, "encrypted-secret")

	enr, err := us.GetTOTPEnrollment(u.ID)
	if err != nil {
		t.Fatalf("GetTOTPEnrollment: %v", err)
	}
	if !enr.Enabled {
		t.Fatal("ConfirmTOTP did not enable the factor")
	}

	n, err := us.CountUnusedRecoveryCodes(u.ID)
	if err != nil {
		t.Fatalf("CountUnusedRecoveryCodes: %v", err)
	}
	if n != len(hashes) {
		t.Fatalf("unused codes = %d, want %d", n, len(hashes))
	}

	// Both read paths must agree that login now needs a second factor.
	byID, _ := us.GetUserByID(u.ID)
	byName, _ := us.GetUserByUsername(u.Username)
	if !byID.TOTPEnabled || !byName.TOTPEnabled {
		t.Fatalf("TOTPEnabled: byID = %v, byUsername = %v", byID.TOTPEnabled, byName.TOTPEnabled)
	}
}

func TestConfirmTOTP_WithoutPendingSecret(t *testing.T) {
	us := store.NewUserStore(testDB)
	u := seedUser(t, "u"+uid())

	err := us.ConfirmTOTP(u.ID, []string{"hash-" + uid()})
	if !errors.Is(err, store.ErrTOTPNotEnrolled) {
		t.Fatalf("err = %v, want ErrTOTPNotEnrolled", err)
	}
}

func TestSetPendingTOTPSecret_RefusesToOverwriteEnabledFactor(t *testing.T) {
	us := store.NewUserStore(testDB)
	u := seedUser(t, "u"+uid())
	enroll(t, us, u.ID, "encrypted-secret")

	err := us.SetPendingTOTPSecret(u.ID, "a-different-secret")
	if !errors.Is(err, store.ErrTOTPAlreadyEnabled) {
		t.Fatalf("err = %v, want ErrTOTPAlreadyEnabled", err)
	}

	enr, _ := us.GetTOTPEnrollment(u.ID)
	if enr.Secret != "encrypted-secret" {
		t.Fatalf("the working secret was replaced: %q", enr.Secret)
	}
}

func TestConsumeRecoveryCode_SingleUse(t *testing.T) {
	us := store.NewUserStore(testDB)
	u := seedUser(t, "u"+uid())
	hashes := enroll(t, us, u.ID, "encrypted-secret")

	ok, err := us.ConsumeRecoveryCode(u.ID, hashes[0])
	if err != nil || !ok {
		t.Fatalf("first use: ok = %v, err = %v", ok, err)
	}

	ok, err = us.ConsumeRecoveryCode(u.ID, hashes[0])
	if err != nil {
		t.Fatalf("second use: %v", err)
	}
	if ok {
		t.Fatal("a recovery code was redeemable twice")
	}

	n, _ := us.CountUnusedRecoveryCodes(u.ID)
	if n != len(hashes)-1 {
		t.Fatalf("unused codes = %d, want %d", n, len(hashes)-1)
	}
}

func TestConsumeRecoveryCode_IsScopedToTheUser(t *testing.T) {
	us := store.NewUserStore(testDB)
	alice := seedUser(t, "u"+uid())
	mallory := seedUser(t, "u"+uid())

	hashes := enroll(t, us, alice.ID, "encrypted-secret")
	enroll(t, us, mallory.ID, "encrypted-secret")

	ok, err := us.ConsumeRecoveryCode(mallory.ID, hashes[0])
	if err != nil {
		t.Fatalf("ConsumeRecoveryCode: %v", err)
	}
	if ok {
		t.Fatal("one user's recovery code was redeemed by another")
	}
}

func TestReplaceRecoveryCodes_RetiresTheOldSet(t *testing.T) {
	us := store.NewUserStore(testDB)
	u := seedUser(t, "u"+uid())
	old := enroll(t, us, u.ID, "encrypted-secret")

	fresh := []string{"new-a-" + uid(), "new-b-" + uid()}
	if err := us.ReplaceRecoveryCodes(u.ID, fresh); err != nil {
		t.Fatalf("ReplaceRecoveryCodes: %v", err)
	}

	if ok, _ := us.ConsumeRecoveryCode(u.ID, old[0]); ok {
		t.Fatal("a retired recovery code is still redeemable")
	}
	if ok, _ := us.ConsumeRecoveryCode(u.ID, fresh[0]); !ok {
		t.Fatal("a freshly issued recovery code was rejected")
	}
}

func TestDisableTOTP_ClearsSecretAndCodes(t *testing.T) {
	us := store.NewUserStore(testDB)
	u := seedUser(t, "u"+uid())
	hashes := enroll(t, us, u.ID, "encrypted-secret")

	if err := us.DisableTOTP(u.ID); err != nil {
		t.Fatalf("DisableTOTP: %v", err)
	}

	if _, err := us.GetTOTPEnrollment(u.ID); !errors.Is(err, store.ErrTOTPNotEnrolled) {
		t.Fatalf("err = %v, want ErrTOTPNotEnrolled", err)
	}
	if n, _ := us.CountUnusedRecoveryCodes(u.ID); n != 0 {
		t.Fatalf("%d recovery codes survived disabling", n)
	}
	if ok, _ := us.ConsumeRecoveryCode(u.ID, hashes[0]); ok {
		t.Fatal("a recovery code survived disabling")
	}

	reloaded, _ := us.GetUserByID(u.ID)
	if reloaded.TOTPEnabled {
		t.Fatal("the user still reports a second factor")
	}
}

func TestDeleteAccountGDPR_RemovesSecondFactor(t *testing.T) {
	us := store.NewUserStore(testDB)
	u := seedUser(t, "u"+uid())
	enroll(t, us, u.ID, "encrypted-secret")

	if _, err := us.DeleteAccountGDPR(u.ID); err != nil {
		t.Fatalf("DeleteAccountGDPR: %v", err)
	}

	// The row is anonymized, so read the columns directly.
	var secret *string
	var enabled bool
	err := testDB.QueryRow(`SELECT totp_secret, totp_enabled FROM users WHERE id = $1`, u.ID).
		Scan(&secret, &enabled)
	if err != nil {
		t.Fatalf("read back user: %v", err)
	}
	if secret != nil {
		t.Fatalf("the TOTP secret survived erasure: %q", *secret)
	}
	if enabled {
		t.Fatal("the account still reports a second factor after erasure")
	}
}
