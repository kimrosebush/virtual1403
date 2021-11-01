package main

// Copyright 2021 Matthew R. Wilson <mwilson@mattwilson.org>
//
// This file is part of virtual1403
// <https://github.com/racingmars/virtual1403>.
//
// virtual1403 is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// virtual1403 is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with virtual1403. If not, see <https://www.gnu.org/licenses/>.

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/racingmars/virtual1403/webserver/db"
	"github.com/racingmars/virtual1403/webserver/mailer"
	"github.com/racingmars/virtual1403/webserver/model"
)

// home serves the home page with the login and signup forms. If the user is
// already logged in, we redirect to the user's personal info page.
func (app *application) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// If the user is currently logged in, go to the user page.
	if app.session.GetString(r, "user") != "" {
		http.Redirect(w, r, "user", http.StatusSeeOther)
		return
	}

	responseVars := make(map[string]interface{})
	responseVars["verifySuccess"] = app.session.Get(r, "verifySuccess")
	if responseVars["verifySuccess"] != nil {
		app.session.Remove(r, "verifySuccess")
	}

	// Otherwise, show the front page.
	app.render(w, r, "home.page.tmpl", responseVars)
}

// login handles user login requests and if successful sets the session cookie
// user value to the logged in user's email address.
func (app *application) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	email := strings.TrimSpace(r.PostFormValue("email"))
	pass := r.PostFormValue("password")

	if email == "" || pass == "" {
		app.renderLoginError(w, r, email, "Must provide email and password.")
		return
	}

	u, err := app.db.GetUser(email)
	if err != nil {
		app.renderLoginError(w, r, email, "Invalid login credentials.")
		return
	}

	if !u.CheckPassword(pass) {
		app.renderLoginError(w, r, email, "Invalid login credentials.")
		return
	}

	app.session.Put(r, "user", u.Email)
	http.Redirect(w, r, "user", http.StatusSeeOther)
}

func (app *application) renderLoginError(w http.ResponseWriter,
	r *http.Request, email, message string) {

	app.render(w, r, "home.page.tmpl", map[string]string{
		"loginEmail": email,
		"loginError": message,
	})
}

func (app *application) renderSignupError(w http.ResponseWriter,
	r *http.Request, email, name, message string) {

	app.render(w, r, "home.page.tmpl", map[string]string{
		"signupEmail": email,
		"signupName":  name,
		"signupError": message,
	})
}

// logout destroys the user's session cookie to log them out and sends them
// back to the home page.
func (app *application) logout(w http.ResponseWriter, r *http.Request) {
	app.session.Destroy(r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// This is the default page for logged-in users
func (app *application) userInfo(w http.ResponseWriter, r *http.Request) {
	// Verify we have a logged in, valid user
	u := app.checkLoggedInUser(r)
	if u == nil {
		// No logged in user
		app.session.Destroy(r)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Get 10 most recent jobs the user printed
	joblog, err := app.db.GetUserJobLog(u.Email, 10)
	if err != nil {
		log.Printf("ERR  db error getting user joblog for %s: %v", u.Email, err)
		// We'll allow the page to render, it'll just have an empty job log
	}

	responseValues := map[string]interface{}{
		"verified":        u.Verified,
		"name":            u.FullName,
		"email":           u.Email,
		"apiKey":          u.AccessKey,
		"apiEndpoint":     app.serverBaseURL + "/print",
		"pageCount":       u.PageCount,
		"jobCount":        u.JobCount,
		"passwordError":   app.session.Get(r, "passwordError"),
		"passwordSuccess": app.session.Get(r, "passwordSuccess"),
		"verifySuccess":   app.session.Get(r, "verifySuccess"),
		"joblog":          joblog,
	}

	if responseValues["passwordError"] != nil {
		app.session.Remove(r, "passwordError")
	}
	if responseValues["passwordSuccess"] != nil {
		app.session.Remove(r, "passwordSuccess")
	}
	if responseValues["verifySuccess"] != nil {
		app.session.Remove(r, "verifySuccess")
	}

	app.render(w, r, "user.page.tmpl", responseValues)
}

// POST hander to regenerate a user's access key
func (app *application) regenkey(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		// Don't accept anything other than a POST
		http.Redirect(w, r, "user", http.StatusSeeOther)
		return
	}

	// Verify we have a logged in, valid user
	u := app.checkLoggedInUser(r)
	if u == nil {
		// No logged in user
		app.session.Destroy(r)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// All checks passed... regenerate the key
	u.GenerateAccessKey()
	err := app.db.SaveUser(*u)
	if err != nil {
		log.Printf("ERROR couldn't save user `%s` in DB: %v", u.Email,
			err)
		app.serverError(w, "Sorry, a database error has occurred")
		return
	}

	log.Printf("INFO  %s generated a new access key", u.Email)
	http.Redirect(w, r, "user", http.StatusSeeOther)
}

// listUsers provides logged-in administrators with a list of all users in the
// database.
func (app *application) listUsers(w http.ResponseWriter, r *http.Request) {
	u := app.checkLoggedInUser(r)
	if u == nil {
		// No logged in user
		app.session.Destroy(r)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Only display this page to administrators
	if !u.Admin {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, "This page is only available to administrators.")
		return
	}

	users, err := app.db.GetUsers()
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}

	log.Printf("INFO  %s accessed the users list page", u.Email)

	app.render(w, r, "users.page.tmpl", users)
}

// listJobs provides logged-in administrators with a list of the 100 most
// recent jobs.
func (app *application) listJobs(w http.ResponseWriter, r *http.Request) {
	u := app.checkLoggedInUser(r)
	if u == nil {
		// No logged in user
		app.session.Destroy(r)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Only display this page to administrators
	if !u.Admin {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, "This page is only available to administrators.")
		return
	}

	jobs, err := app.db.GetJobLog(100)
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}

	log.Printf("INFO  %s accessed the job log page", u.Email)

	app.render(w, r, "jobs.page.tmpl", jobs)
}

// signup is the HTTP POST handler for /signup, to create new user accounts.
// If everything is okay, we will create the new user in an unverified state
// and send the new email address the verification email.
func (app *application) signup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	email := r.PostFormValue("email")
	name := r.PostFormValue("name")
	botDetection := r.PostFormValue("website")
	password := r.PostFormValue("password")
	passwordConfirm := r.PostFormValue("password-confirm")

	if botDetection != "" {
		http.Error(w, "Go away, bot", http.StatusForbidden)
		return
	}

	if !mailer.ValidateAddress(strings.ToLower(email)) {
		app.renderSignupError(w, r, email, name,
			"Must provide a valid email address.")
		return
	}

	if name == "" {
		app.renderSignupError(w, r, email, name,
			"Must provide a name.")
		return
	}

	if password == "" || len(password) < 8 {
		app.renderSignupError(w, r, email, name,
			"Must provide a password at least 8 characters long.")
		return
	}

	if password != passwordConfirm {
		app.renderSignupError(w, r, email, name,
			"Passwords do not match.")
		return
	}

	if _, err := app.db.GetUser(email); err != db.ErrNotFound {
		app.renderSignupError(w, r, email, name,
			"That email address already has an account.")
		return
	}

	newuser := model.NewUser(email, password)
	newuser.FullName = name
	newuser.Enabled = true

	if err := app.db.SaveUser(newuser); err != nil {
		log.Printf("ERROR couldn't save new user %s to DB: %v", email, err)
		app.serverError(w, "Unexpected error saving new user to database.")
		return
	}

	app.session.Put(r, "user", newuser.Email)

	if err := mailer.SendVerificationCode(app.mailconfig, newuser.Email,
		app.serverBaseURL+"/verify?token="+
			url.QueryEscape(newuser.AccessKey)); err != nil {
		log.Printf("ERROR couldn't send verification email for %s: %v",
			newuser.Email, err)
	}

	http.Redirect(w, r, "user", http.StatusSeeOther)
}

// changePassword is the HTTP handler for POSTS to /changepassword for users
// to change their password. We verify that a valid user is logged in, then
// change the password if the old password checks out.
func (app *application) changePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Verify we have a logged in, valid user
	u := app.checkLoggedInUser(r)
	if u == nil {
		// No logged in user
		app.session.Destroy(r)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if !u.CheckPassword(r.PostFormValue("password")) {
		// Users existing password does not match
		app.session.Put(r, "passwordError",
			"Your current password was incorrect.")
		log.Printf("INFO  %s unsuccessfully attempted password change", u.Email)
		http.Redirect(w, r, "user", http.StatusSeeOther)
		return
	}

	newPassword := r.PostFormValue("new-password")
	newPassword2 := r.PostFormValue("new-password2")

	if len(newPassword) < 8 {
		app.session.Put(r, "passwordError",
			"Your new password must be 8 or more characters long.")
		log.Printf("INFO  %s unsuccessfully attempted password change", u.Email)
		http.Redirect(w, r, "user", http.StatusSeeOther)
		return
	}

	if newPassword != newPassword2 {
		app.session.Put(r, "passwordError",
			"New passwords do not match.")
		log.Printf("INFO  %s unsuccessfully attempted password change", u.Email)
		http.Redirect(w, r, "user", http.StatusSeeOther)
		return
	}

	u.SetPassword(newPassword)
	if err := app.db.SaveUser(*u); err != nil {
		app.serverError(w, "Sorry, a database error has occurred")
		return
	}

	app.session.Put(r, "passwordSuccess",
		"Your password was successfully changed.")
	log.Printf("INFO  %s successfully changed their password", u.Email)
	http.Redirect(w, r, "user", http.StatusSeeOther)
}

// verifyUser is the HTTP hander for /verify; we expect /verify?token=... to
// verify a user after sending them the email verification link. If the
// verification token belongs to an unverified account, we will set the
// account to verified and generate a new token (which will be used as their
// print API access token going forward).
func (app *application) verifyUser(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.NotFound(w, r)
		return
	}

	u, err := app.db.GetUserForAccessKey(token)
	if err == db.ErrNotFound {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "That token was not found or has already been "+
			"used to verify the user.")
		return
	}
	if err != nil {
		app.serverError(w, "Unexpected error looking up token")
		return
	}

	if u.Verified {
		w.WriteHeader(http.StatusConflict)
		io.WriteString(w, "That user has already been verified.")
		return
	}

	u.Verified = true
	u.GenerateAccessKey()

	if err := app.db.SaveUser(u); err != nil {
		app.serverError(w, "Error saving user record after verification")
		return
	}

	app.session.Put(r, "verifySuccess", "Email address successfully verified.")
	log.Printf("INFO  %s verified their account", u.Email)
	http.Redirect(w, r, "user", http.StatusSeeOther)
}

// checkLoggedInUser checks if a user is logged in to the session in the HTTP
// request r. If there is a valid session cookie with a username, we check
// that the user still exists and isn't disabled. If everything is good, we
// return a pointer to the logged-in user. If the user isn't logged in or
// there is a problem, we return nil.
func (app *application) checkLoggedInUser(r *http.Request) *model.User {
	username := app.session.GetString(r, "user")
	if username == "" {
		return nil
	}

	user, err := app.db.GetUser(username)
	if err == db.ErrNotFound {
		log.Printf(
			"INFO  user `%s` has a session cookie but the account no "+
				"longer exists",
			username)
		return nil
	}
	if err != nil {
		log.Printf("ERROR couldn't look up user `%s` in DB: %v", username,
			err)
		return nil
	}

	if !user.Enabled {
		log.Printf(
			"INFO  user `%s` has a session cookie but the account is disabled",
			username)
		return nil
	}

	// At this point, we have a valid, active logged-in user.
	return &user
}