package main

import (
	"os"
	"os/exec"
	"log"
	"fmt"
	"time"
	"strings"
	"bytes"
	"net/http"
	"math/rand"
	"golang.org/x/crypto/bcrypt"
	"github.com/julienschmidt/httprouter"
	"github.com/nicksnyder/go-i18n/i18n"
	"github.com/jung-kurt/gofpdf"
)

func PasswordURL() string {
	return Base_URL + "password"
}

func PasswordBcrypt(_, password string) string {
	pswd := []byte(password)
	hash, _ := bcrypt.GenerateFromPassword(pswd, bcrypt.DefaultCost)
	return string(hash)
}

func PasswordSha512(address, password string) string {
	cmd := exec.Command("doveadm", "pw", "-s", "SHA512-CRYPT", "-u", address, "-p", password)
	out := bytes.Buffer{}
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		log.Printf("FATAL PasswordEncrypt:Run: %s", err)
		os.Exit(1)
	}
	hash := strings.TrimSpace(strings.TrimPrefix(out.String(), "{SHA512-CRYPT}"))
	return hash
}

func PasswordRandom(length int) string {
	seed := rand.New(rand.NewSource(time.Now().UnixNano()))
	cset := "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789"

	buff := make([]byte, length)
	for index := range buff {
		buff[index] = cset[seed.Intn(len(cset))]
	}

	return string(buff)
}

func PasswordLetter(w http.ResponseWriter, ctx Context, initial string) {
	t, _ := i18n.Tfunc(Language)

	title := fmt.Sprintf(t("address_email_subject"), ctx.Address.Email)

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetTitle(title, true)
	pdf.AddPage()

	pdf.SetFont("arial", "B", 14)
	pdf.Write(14, title + "\n")

	pdf.SetFont("arial", "", 14)
	pdf.Write(14, t("password_email_initial"))

	pdf.SetFont("courier", "B", 14)
	pdf.Write(14, initial + "\n")

	pdf.SetFont("arial", "", 14)
	pdf.Write(14, t("password_email_info"))

	if err := pdf.Output(w); err != nil {
		log.Printf("ERROR PasswordLetter:Output: %s", err)
	}
}

func PasswordEdit(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	log.Printf("INFO  GET %s", PasswordURL())

	db := OpenDB(true)
	defer CloseDB()

	ctx := AddressContext(w, r, "password_edit", false, db)
	if !ctx.LoggedIn {
		return
	}

	RenderHtml(w, r, "password_edit", ctx)
}

func PasswordUpdate(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	t, _ := i18n.Tfunc(Language)
	log.Printf("INFO  POST %s", PasswordURL())

	db := OpenDB(true)
	defer CloseDB()

	ctx := AddressContext(w, r, "password_update", false, db)
	if !ctx.LoggedIn {
		return
	}

	password     := r.FormValue("password_password")
	confirmation := r.FormValue("password_confirmation")

	if password == "" {
		flash := t("flash_missing_password")
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, PasswordURL(), http.StatusFound)
		return
	}
	if password != confirmation {
		flash := t("flash_bad_confirmation")
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, PasswordURL(), http.StatusFound)
		return
	}

	update := make(map[string]interface{})
	update["bcrypt"] = PasswordBcrypt(ctx.CurrentAddress.Email, password)
	update["sha512"] = PasswordSha512(ctx.CurrentAddress.Email, password)
	update["updated_at"] = time.Now()
	update["updated_by"] = ctx.CurrentAddress.ID

	if err := db.Model(ctx.CurrentAddress).Updates(update).Error; err != nil {
		flash := fmt.Sprintf(t("flash_error_text"), err.Error())
		SetFlash(w, F_ERROR, flash)
		http.Redirect(w, r, LogoutURL(), http.StatusFound)
		return
	}

	flash := fmt.Sprintf(t("flash_updated"), t("address_password"))
	SetFlash(w, F_INFO, flash)
	http.Redirect(w, r, LogoutURL(), http.StatusFound)
}
