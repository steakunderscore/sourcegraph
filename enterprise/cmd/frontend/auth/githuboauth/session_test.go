package githuboauth

import (
	"context"
	"net/url"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	githublogin "github.com/dghubble/gologin/github"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/auth"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/db"
	"github.com/sourcegraph/sourcegraph/pkg/actor"
	"github.com/sourcegraph/sourcegraph/pkg/extsvc"
	githubsvc "github.com/sourcegraph/sourcegraph/pkg/extsvc/github"
	"golang.org/x/oauth2"
)

func init() {
	spew.Config.DisablePointerAddresses = true
	spew.Config.SortKeys = true
	spew.Config.SpewKeys = true
}

func TestGetOrCreateUser(t *testing.T) {
	ghURL, _ := url.Parse("https://github.com")
	codeHost := extsvc.NewCodeHost(ghURL, githubsvc.ServiceType)
	clientID := "client-id"

	// Top-level mock data
	//
	// authSaveableUsers that will be accepted by auth.GetAndSaveUser
	authSaveableUsers := map[string]int32{
		"alice": 1,
	}

	type input struct {
		description     string
		ghUser          *github.User
		ghUserEmails    []*githubsvc.UserEmail
		ghUserEmailsErr error
		allowSignup     bool
	}
	cases := []struct {
		inputs        []input
		expActor      *actor.Actor
		expErr        bool
		expAuthUserOp *auth.GetAndSaveUserOp
	}{
		{
			inputs: []input{{
				description: "ghUser, verified email -> session created",
				ghUser:      &github.User{ID: github.Int64(101), Login: github.String("alice")},
				ghUserEmails: []*githubsvc.UserEmail{{
					Email:    "alice@example.com",
					Primary:  true,
					Verified: true,
				}},
			}},
			expActor: &actor.Actor{UID: 1},
			expAuthUserOp: &auth.GetAndSaveUserOp{
				UserProps:       u("alice", "alice@example.com", true),
				ExternalAccount: acct("github", "https://github.com/", clientID, "101"),
			},
		},
		{
			inputs: []input{{
				description: "ghUser, primary email not verified but another is -> no session created",
				ghUser:      &github.User{ID: github.Int64(101), Login: github.String("alice")},
				ghUserEmails: []*githubsvc.UserEmail{{
					Email:    "alice@example1.com",
					Primary:  true,
					Verified: false,
				}, {
					Email:    "alice@example2.com",
					Primary:  false,
					Verified: false,
				}, {
					Email:    "alice@example3.com",
					Primary:  false,
					Verified: true,
				}},
			}},
			expActor: &actor.Actor{UID: 1},
			expAuthUserOp: &auth.GetAndSaveUserOp{
				UserProps:       u("alice", "alice@example3.com", true),
				ExternalAccount: acct("github", "https://github.com/", clientID, "101"),
			},
		},
		{
			inputs: []input{{
				description: "ghUser, no emails -> no session created",
				ghUser:      &github.User{ID: github.Int64(101), Login: github.String("alice")},
			}, {
				description:     "ghUser, email fetching err -> no session created",
				ghUser:          &github.User{ID: github.Int64(101), Login: github.String("alice")},
				ghUserEmailsErr: errors.New("x"),
			}, {
				description: "ghUser, plenty of emails but none verified -> no session created",
				ghUser:      &github.User{ID: github.Int64(101), Login: github.String("alice")},
				ghUserEmails: []*githubsvc.UserEmail{{
					Email:    "alice@example1.com",
					Primary:  true,
					Verified: false,
				}, {
					Email:    "alice@example2.com",
					Primary:  false,
					Verified: false,
				}, {
					Email:    "alice@example3.com",
					Primary:  false,
					Verified: false,
				}},
			}, {
				description: "no ghUser -> no session created",
			}, {
				description: "ghUser, verified email, unsaveable -> no session created",
				ghUser:      &github.User{ID: github.Int64(102), Login: github.String("bob")},
			}},
			expErr: true,
		},
	}
	for _, c := range cases {
		for _, ci := range c.inputs {
			c, ci := c, ci
			t.Run(ci.description, func(t *testing.T) {
				githubsvc.MockGetAuthenticatedUserEmails = func(ctx context.Context, token string) ([]*githubsvc.UserEmail, error) {
					return ci.ghUserEmails, ci.ghUserEmailsErr
				}
				var gotAuthUserOp *auth.GetAndSaveUserOp
				auth.MockGetAndSaveUser = func(ctx context.Context, op auth.GetAndSaveUserOp) (userID int32, safeErrMsg string, err error) {
					if gotAuthUserOp != nil {
						t.Fatal("GetAndSaveUser called more than once")
					}
					op.ExternalAccountData = extsvc.ExternalAccountData{} // ignore ExternalAccountData value
					gotAuthUserOp = &op

					if uid, ok := authSaveableUsers[op.UserProps.Username]; ok {
						return uid, "", nil
					}
					return 0, "safeErr", errors.New("auth.GetAndSaveUser error")
				}
				defer func() {
					auth.MockGetAndSaveUser = nil
					githubsvc.MockGetAuthenticatedUserEmails = nil
				}()

				ctx := githublogin.WithUser(context.Background(), ci.ghUser)
				s := &sessionIssuerHelper{
					CodeHost:    codeHost,
					clientID:    clientID,
					allowSignup: ci.allowSignup,
				}
				tok := &oauth2.Token{AccessToken: "dummy-value-that-isnt-relevant-to-unit-correctness"}
				actr, _, err := s.GetOrCreateUser(ctx, tok)
				if got, exp := actr, c.expActor; !reflect.DeepEqual(got, exp) {
					t.Errorf("expected actor %v, got %v", exp, got)
				}
				if c.expErr && err == nil {
					t.Errorf("expected err %v, but was nil", c.expErr)
				} else if !c.expErr && err != nil {
					t.Errorf("expected no error, but was %v", err)
				}
				if got, exp := gotAuthUserOp, c.expAuthUserOp; !reflect.DeepEqual(got, exp) {
					dmp := diffmatchpatch.New()
					t.Errorf("auth.GetOrCreateUser(op) got != exp, diff(got, exp):\n%s",
						dmp.DiffPrettyText(dmp.DiffMain(spew.Sdump(exp), spew.Sdump(got), false)))
				}
			})
		}
	}
}

func u(username, email string, emailIsVerified bool) db.NewUser {
	return db.NewUser{
		Username:        username,
		Email:           email,
		EmailIsVerified: emailIsVerified,
	}
}

func acct(serviceType, serviceID, clientID, accountID string) extsvc.ExternalAccountSpec {
	return extsvc.ExternalAccountSpec{
		ServiceType: serviceType,
		ServiceID:   serviceID,
		ClientID:    clientID,
		AccountID:   accountID,
	}
}
