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

func LoginURL() string {
	return Base_URL + "login"
}

func LogoutURL() string {
	return Base_URL + "logout"
}

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

	//dial := gomail.NewDialer(SMTP_Host, SMTP_Port, SMTP_Username, SMTP_Password)
	dial := gomail.Dialer{Host: "localhost", Port: 25}
	if err := dial.DialAndSend(mail); err != nil {
		log.Printf("ERROR LoginEmail:DialAndSend: %s", err)
		return err
	}

	return nil
}

func LoginLoginGet(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	log.Printf("INFO  GET %s", LoginURL())

	ctx := Context{Title: "login_title", Base_URL: Base_URL}

	RenderHtml(w, r, "login", ctx)
}

func LoginLoginPost(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	log.Printf("INFO  POST %s", LoginURL())

	db := OpenDB(true)
	defer CloseDB()

	email    := r.FormValue("login_email")
	password := r.FormValue("login_password")
	submit   := r.FormValue("login_action")
	log.Printf("DEBUG email='%s' password=[hidden] submit='%s'", email, submit)

	address := AddressFindByEmail(email, db)
	if address == nil {
		log.Printf("DEBUG Login: address %s unknown", email)
		SetFlash(w, F_ERROR, t("flash_login_failure"))
		http.Redirect(w, r, LoginURL(), http.StatusFound)
		return
	}
	log.Printf("DEBUG Login: found %d = %s", address.ID, address.Email)

	if submit == "reset" {
		log.Printf("DEBUG Login: reset %s", address.Email)
		if address.OtherEmail != "" {
			if err := LoginEmail(address, db); err != nil {
				flash := fmt.Sprintf(t("flash_error_text"), err.Error())
				SetFlash(w, F_ERROR, flash)
				http.Redirect(w, r, LoginURL(), http.StatusFound)
				return
			}
			SetFlash(w, F_INFO, t("flash_check_other_email"))
			http.Redirect(w, r, LoginURL(), http.StatusFound)
			return
		}
		SetFlash(w, F_INFO, t("flash_use_password_letter"))
		http.Redirect(w, r, LoginURL(), http.StatusFound)
		return
	}

	err_i := bcrypt.CompareHashAndPassword([]byte(address.Initial), []byte(password))
	log.Printf("DEBUG Login: Initial=%v", err_i)
	err_p := bcrypt.CompareHashAndPassword([]byte(address.Bcrypt),  []byte(password))
	log.Printf("DEBUG Login: Password=%v", err_p)

	if err_i == nil || (err_p == nil && address.Admin == false) {
		log.Printf("DEBUG Login: send to PasswordURL")
		uid := fmt.Sprintf("%d", address.ID)
		SetCookie(w, "address_id",  uid)
		SetFlash(w, F_INFO, t("flash_login_update"))
		http.Redirect(w, r, PasswordURL(), http.StatusFound)
		return
	}

	if err_p == nil && address.Admin == true {
		log.Printf("DEBUG Login: send to HomeURL")
		uid := fmt.Sprintf("%d", address.ID)
		SetCookie(w, "address_id",  uid)
		SetFlash(w, F_INFO, t("flash_login_success"))
		http.Redirect(w, r, HomeURL(), http.StatusFound)
		return
	}

	log.Printf("DEBUG Login: bad password for %s", address.Email)
	SetFlash(w, F_ERROR, t("flash_login_failure"))
	http.Redirect(w, r, LoginURL(), http.StatusFound)
}

func LoginLogout(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	log.Printf("INFO  GET %s", LogoutURL())

	uid := GetCookie(r, "address_id")
	DelCookie(w, "address_id")
	if uid != "" {
		SetFlash(w, F_INFO, t("flash_logout_bye"))
	}
	DelCookie(w, "referer")

	http.Redirect(w, r, LoginURL(), http.StatusFound)
}
