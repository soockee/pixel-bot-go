package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/soocke/pixel-bot-go/internal/cloud"
)

// accountPath keeps the Google refresh token, DPAPI-encrypted, beside the config.
const accountPath = "pixel_bot_account.bin"

var errNoSignIn = errors.New("sign-in is not configured: set cloud_url and oauth_client_id")

// newCloud returns the upload service client, or nil while it is not configured.
func (a *app) newCloud() *cloud.Client {
	cfg := a.container.Config
	url, id, secret := cmp.Or(cfg.CloudURL, cloud.DefaultURL), cfg.OAuthClientID, cfg.OAuthClientSecret
	if id == "" {
		id, secret = cloud.DefaultClientID, cloud.DefaultClientSecret
	}
	if url == "" || id == "" {
		return nil
	}
	return cloud.New(url, id, secret)
}

// can reports whether the signed-in account's role grants permission. It runs
// on the loop.
func (a *app) can(permission string) bool {
	return a.session != nil && a.session.Account.Can(permission)
}

// setSession shows s, or signed out when s is nil. It runs on the loop.
func (a *app) setSession(s *cloud.Session) {
	a.session = s
	view := a.container.View
	if s == nil {
		view.SetAccount("", "", false, false)
	} else {
		view.SetAccount(s.Account.Email, s.Account.Role, s.Account.Can(cloud.PermFish), s.Account.Can(cloud.PermUpload))
	}
}

// restoreAccount signs in with the kept refresh token, if there is one.
// Without a configured sign-in, fishing needs no role.
func (a *app) restoreAccount() {
	a.container.View.SetSignIn(a.cloud != nil)
	if a.cloud == nil {
		a.container.View.SetAccount("", "", true, false)
		return
	}
	token, err := cloud.LoadToken(accountPath)
	if err != nil || token == "" {
		if err != nil {
			a.logger.Warn("kept sign-in unreadable", "error", err)
		}
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		s, err := a.cloud.Open(ctx, token)
		a.loop.Do(func() {
			if err != nil {
				a.logger.Warn("sign-in restore failed", "error", err)
				a.container.View.SetStatus("Sign in again: " + err.Error())
				return
			}
			a.logger.Info("signed in", "email", s.Account.Email, "role", s.Account.Role)
			a.setSession(s)
		})
	}()
}

// signIn runs Google sign-in in the browser and keeps the refresh token.
func (a *app) signIn() error {
	err := a.trySignIn()
	if err != nil {
		a.logger.Warn("sign-in failed", "error", err)
		a.loop.Do(func() { a.container.View.SetStatus("Sign-in failed: " + err.Error()) })
	}
	return err
}

func (a *app) trySignIn() error {
	if a.cloud == nil {
		return errNoSignIn
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	token, err := a.cloud.SignIn(ctx, a.wails.Browser.OpenURL)
	if err != nil {
		return err
	}
	s, err := a.cloud.Open(ctx, token)
	if err != nil {
		return err
	}
	if err := cloud.SaveToken(accountPath, token); err != nil {
		a.logger.Warn("sign-in not kept", "error", err)
	}
	a.logger.Info("signed in", "email", s.Account.Email, "role", s.Account.Role)
	a.loop.Do(func() {
		a.setSession(s)
		a.container.View.SetStatus("Signed in as " + s.Account.Email)
	})
	return nil
}

// signOut forgets the account and its kept token.
func (a *app) signOut() {
	if err := cloud.ForgetToken(accountPath); err != nil {
		a.logger.Warn("kept sign-in not removed", "error", err)
	}
	a.loop.Do(func() {
		a.setSession(nil)
		a.container.View.SetStatus("Signed out")
	})
}

// upload sends the finished recording in dir, showing progress in the status bar.
func (a *app) upload(dir, fingerprint string, frames, audio bool) error {
	name := filepath.Base(dir)
	var s *cloud.Session
	var refusal error
	a.loop.Do(func() {
		st := a.container.View.State()
		switch {
		case !a.can(cloud.PermUpload):
			refusal = errors.New("sign in with an account whose role may upload recordings")
		case st.Uploading != "":
			refusal = errors.New("an upload is running")
		case st.RecordingName == name:
			refusal = errors.New("stop the recording before uploading it")
		default:
			s = a.session
			a.container.View.SetUploading(name)
			a.container.View.SetStatus("Uploading " + name)
		}
	})
	if refusal != nil {
		return refusal
	}

	err := s.UploadReviewed(context.Background(), dir, fingerprint, frames, audio, func(done, total int) {
		a.loop.Do(func() {
			a.container.View.SetStatus(fmt.Sprintf("Uploading %s: %d of %d files", name, done, total))
		})
	})
	a.loop.Do(func() {
		a.container.View.SetUploading("")
		if err != nil {
			a.container.View.SetStatus("Upload failed: " + err.Error())
			return
		}
		a.container.View.SetStatus("Uploaded " + name)
	})
	if err != nil {
		a.logger.Error("upload failed", "recording", name, "error", err)
		return err
	}
	a.logger.Info("recording uploaded", "recording", name)
	return nil
}
