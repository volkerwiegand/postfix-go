package main

import (
	"log"
	"fmt"
	"io"
	"time"
	"net/http"
	"golang.org/x/crypto/bcrypt"
	"github.com/julienschmidt/httprouter"
	"github.com/jinzhu/gorm"
	"github.com/nicksnyder/go-i18n/i18n"
	"gopkg.in/gomail.v2"
)

const (
	LoginURL  = "login"
	LogoutURL = "logout"
)

func LoginEmail(address *Address, db *gorm.DB) error {
	t, _ := i18n.Tfunc(Language)

	initial := PasswordRandom(10)
	update := make(map[string]interface{})
	update["initial"] = PasswordBcrypt(address.Email, initial)
	update["updated_at"] = time.Now()
	update["updated_by"] = address.ID

	if err := db.Model(address).Updates(update).Error; err != nil {
		log.Printf("ERROR LoginEmail:Updates: %s", err)
		return err
	}

	mail := gomail.NewMessage()
	mail.SetHeader("From",    address.Email)
	mail.SetHeader("To",      address.OtherEmail)
	mail.SetHeader("Subject", fmt.Sprintf(t("address_email_subject"), address.Email))
	address.Initial = initial

	tmpl := fmt.Sprintf("password_email_%s", Language)
	mail.AddAlternativeWriter("text/plain", func(w io.Writer) error {
		return Templates.ExecuteTemplate(w, tmpl, address)
	})

	dial := gomail.NewDialer(SMTP_Host, SMTP_Port, SMTP_Username, SMTP_Password)
	if err := dial.DialAndSend(mail); err != nil {
		log.Printf("ERROR LoginEmail:DialAndSend: %s", err)
		return err
	}

	return nil
}

func LoginLoginGet(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	log.Printf("INFO  GET /login")
	prefix := ""

	ctx := Context{Title: "login_title", Prefix: prefix}

	RenderHtml(w, r, "login", ctx)
}

func LoginLoginPost(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	log.Printf("INFO  POST /login")
	prefix := ""

	db := OpenDB(true)
	defer CloseDB()

	email    := r.FormValue("login_email")
	password := r.FormValue("login_password")
	submit   := r.FormValue("login_action")
	log.Printf("DEBUG email='%s' password='%s' submit='%s'", email, "[hidden]", submit)

	address := AddressFindByEmail(email, db)
	if address == nil {
		SetFlash(w, F_ERROR, t("flash_login_failure"))
		http.Redirect(w, r, prefix + LoginURL, http.StatusFound)
		return
	}

	if submit == "reset" {
		log.Printf("DEBUG reset %s", address.Email)
		if address.OtherEmail != "" {
			if err := LoginEmail(address, db); err != nil {
				flash := fmt.Sprintf(t("flash_error_text"), err.Error())
				SetFlash(w, F_ERROR, flash)
				http.Redirect(w, r, prefix + LoginURL, http.StatusFound)
				return
			}
			SetFlash(w, F_INFO, t("flash_check_other_email"))
			http.Redirect(w, r, prefix + LoginURL, http.StatusFound)
			return
		}
		SetFlash(w, F_INFO, t("flash_use_password_letter"))
		http.Redirect(w, r, prefix + LoginURL, http.StatusFound)
		return
	}

	err_f := bcrypt.CompareHashAndPassword([]byte(address.Initial), []byte(password))
	err_p := bcrypt.CompareHashAndPassword([]byte(address.Bcrypt),  []byte(password))

	if err_f == nil || (err_p == nil && address.Admin == false) {
		uid := fmt.Sprintf("%d", address.ID)
		SetCookie(w, "address_id",  uid)
		SetFlash(w, F_INFO, t("flash_login_update"))
		http.Redirect(w, r, prefix + PasswordURL, http.StatusFound)
		return
	}

	if err_p == nil && address.Admin == true {
		uid := fmt.Sprintf("%d", address.ID)
		SetCookie(w, "address_id",  uid)
		SetFlash(w, F_INFO, t("flash_login_success"))
		http.Redirect(w, r, prefix + HomeURL, http.StatusFound)
		return
	}

	log.Printf("DEBUG login failed %s", address.Email)
	SetFlash(w, F_ERROR, t("flash_login_failure"))
	http.Redirect(w, r, prefix + LoginURL, http.StatusFound)
}

func LoginLogout(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	log.Printf("INFO  GET /logout")
	prefix := ""

	uid := GetCookie(r, "address_id")
	DelCookie(w, "address_id")
	if uid != "" {
		SetFlash(w, F_INFO, t("flash_logout_bye"))
	}
	DelCookie(w, "referer")

	http.Redirect(w, r, prefix + LoginURL, http.StatusFound)
}
