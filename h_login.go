package main

import (
	"log"
	"fmt"
	"net/http"
	"golang.org/x/crypto/bcrypt"
	"github.com/julienschmidt/httprouter"
	"github.com/nicksnyder/go-i18n/i18n"
)

const (
	LoginURL  = "/login"
	LogoutURL = "/logout"
)

func LoginLoginGet(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	log.Printf("INFO  GET /login")

	ctx := Context{Title: "login_title"}

	RenderHtml(w, r, "login", ctx)
}

func LoginLoginPost(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	log.Printf("INFO  POST /login")

	db := OpenDB(true)
	defer CloseDB()

	email    := r.FormValue("login_email")
	password := r.FormValue("login_password")

	// TODO reset password if logged in with FirstPass

	if record := AddressFindByEmail(email, db); record != nil {
		hash := []byte(record.Password)
		pswd := []byte(password)
		if err := bcrypt.CompareHashAndPassword(hash, pswd); err == nil {
			uid := fmt.Sprintf("%d", record.ID)
			SetCookie(w, "address_id",  uid)
			SetFlash(w, F_INFO, t("flash_login_success"))
			http.Redirect(w, r, HomeURL, http.StatusFound)
			return
		}
	}

	SetFlash(w, F_ERROR, t("flash_login_failure"))
	http.Redirect(w, r, LoginURL, http.StatusFound)
}

func LoginLogout(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	log.Printf("INFO  GET /logout")

	uid := GetCookie(r, "address_id")
	DelCookie(w, "address_id")
	if uid != "" {
		SetFlash(w, F_INFO, t("flash_logout_bye"))
	}
	DelCookie(w, "referer")

	http.Redirect(w, r, LoginURL, http.StatusFound)
}
