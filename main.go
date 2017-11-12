package main

import (
	"os"
	"fmt"
	"log"
	"time"
	"sync"
	"net/http"
	"html/template"
	"encoding/base64"
	"encoding/json"
	"github.com/julienschmidt/httprouter"
	"github.com/gorilla/csrf"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	_ "github.com/go-sql-driver/mysql"
	"github.com/nicksnyder/go-i18n/i18n"
	"github.com/spf13/viper"
)

const (
	F_INFO  = "info"
	F_ERROR = "error"
)

type Context struct {
	Title          string
	Language       string
	Web_Root       string
	CsrfField      template.HTML
	StyleSheets    []string
	JavaScripts    []string
	Flash          string
	CurrentAddress *Address
	LoggedIn       bool
	Domains        []Domain
	Domain         *Domain
	Addresses      []Address
	Address        *Address
	Aliases        []Alias
	Alias          *Alias
}

var (
	Language      string
	DB_Type       string
	DB_Connect    string
	DB_ConnStr    string
	Web_Addr      string
	Web_Root      string
	Web_Token     string
	Def_Domain    string
	SMTP_Host     string
	SMTP_Port     int
	SMTP_Username string
	SMTP_Password string
	ProdMode      bool
	Verbose       bool
	Templates     *template.Template
	Database      *gorm.DB
	CookiePrefix  = "postfix_go_"
	DB_Mutex      = &sync.Mutex{}
)

func main() {
	//
	// Read the configuration
	//
	viper.SetConfigName("config")
	viper.AddConfigPath("/etc/postfix-go/")
	viper.AddConfigPath("$HOME/.postfix-go")
	viper.AddConfigPath(".")

	viper.SetEnvPrefix("postfix-go")
	viper.AutomaticEnv()

	viper.SetDefault("Language",      "de")
	viper.SetDefault("DB_Type",       "sqlite3")
	viper.SetDefault("Web_Addr",      ":8000")
	viper.SetDefault("Web_Root",      "/postfix-go")
	viper.SetDefault("DB_Connect",    "postfix-go.sql")
	viper.SetDefault("Web_Token",     "_Postfix_Dovecot_Golang_PureCSS_")	// 32 bytes
	viper.SetDefault("Def_Domain",    "example.com")
	viper.SetDefault("SMTP_Host",     "mail.example.com")
	viper.SetDefault("SMTP_Port",     587)
	viper.SetDefault("SMTP_Username", "relay_user")
	viper.SetDefault("SMTP_Password", "relay_pswd")
	viper.SetDefault("ProdMode",      false)
	viper.SetDefault("Verbose",       true)

	if err := viper.ReadInConfig(); err != nil {
		log.Printf("FATAL ReadInConfig: %s", err)
		os.Exit(1)
	}

	Language      = viper.GetString("Language")
	DB_Type       = viper.GetString("DB_Type")
	DB_Connect    = viper.GetString("DB_Connect")
	Web_Addr      = viper.GetString("Web_Addr")
	Web_Root      = viper.GetString("Web_Root")
	Web_Token     = viper.GetString("Web_Token")
	Def_Domain    = viper.GetString("Def_Domain")
	SMTP_Host     = viper.GetString("SMTP_Host")
	SMTP_Port     = viper.GetInt("SMTP_Port")
	SMTP_Username = viper.GetString("SMTP_Username")
	SMTP_Password = viper.GetString("SMTP_Password")
	ProdMode      = viper.GetBool("ProdMode")
	Verbose       = viper.GetBool("Verbose")

	if DB_Type == "mysql" {
		DB_ConnStr = DB_Connect + "?charset=utf8&parseTime=True&loc=Local"
	} else {
		DB_ConnStr = DB_Connect
	}

	if Verbose {
		log.Printf("DEBUG Language ........... %s",        Language)
		log.Printf("DEBUG DB-Connect ......... %s:%s",     DB_Type, DB_ConnStr)
		log.Printf("DEBUG Web-Addr / Root .... %s / '%s'", Web_Addr, Web_Root)
		log.Printf("DEBUG SMTP-Host:Port ..... %s:%d",     SMTP_Host, SMTP_Port)
		log.Printf("DEBUG SMTP-Login ......... %s / %s",   SMTP_Username, SMTP_Password)
	}

	//
	// Setup i18n
	//
	langfile := fmt.Sprintf("locales/%s.all.json", Language)
	if err := i18n.LoadTranslationFile(langfile); err != nil {
		log.Printf("FATAL Language %s: %s", Language, err)
		os.Exit(1)
	}

	//
	// Initialize Database Tables (AddressInit must be last)
	//
	HomeInit()
	LoginInit()
	PasswordInit()
	DomainInit()
	AliasInit()
	AddressInit()

	//
	// Initialize templates and function map
	//
	funcMap := template.FuncMap{
		"safe": func(s string) template.HTML {
			return template.HTML(s)
		},
		"T": func(s string) string {
			t, _ := i18n.Tfunc(Language)
			return t(s)
		},
		"time": func(tm time.Time) string {
			t, _ := i18n.Tfunc(Language)
			return tm.Format(t("date_time"))
		},
	}
	Templates = template.Must(template.New("").Funcs(funcMap).ParseGlob("./templates/*"))

	//
	// Setup the web server and router
	//
	r := httprouter.New()

	r.ServeFiles(Web_Root + "/static/*filepath", http.Dir("./static"))

	r.GET(Web_Root + "/",                   HomeIndex)
	r.GET(Web_Root + "/login",              LoginLoginGet)
	r.POST(Web_Root + "/login",             LoginLoginPost)
	r.GET(Web_Root + "/logout",             LoginLogout)
	r.GET(Web_Root + "/help/:page",         HelpShow)
	r.GET(Web_Root + "/domain",             DomainCreate)
	r.GET(Web_Root + "/domain/:id",         DomainEdit)
	r.POST(Web_Root + "/domain/:id",        DomainUpdate)
	r.GET(Web_Root + "/domain/:id/delete",  DomainDelete)
	r.GET(Web_Root + "/address",            AddressCreate)
	r.GET(Web_Root + "/address/:id",        AddressEdit)
	r.POST(Web_Root + "/address/:id",       AddressUpdate)
	r.GET(Web_Root + "/address/:id/print",  AddressPrint)
	r.GET(Web_Root + "/address/:id/delete", AddressDelete)
	r.GET(Web_Root + "/password",           PasswordEdit)
	r.POST(Web_Root + "/password",          PasswordUpdate)
	// TODO audit trail

	srv := &http.Server{
		Addr:         Web_Addr,
		Handler:      csrf.Protect([]byte(Web_Token), csrf.Secure(ProdMode))(r),
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}

func RenderHtml(w http.ResponseWriter, r *http.Request, tmpl string, ctx Context) {
	ctx.Language = Language
	ctx.Web_Root = Web_Root

	if ctx.Flash = GetCookie(r, "flash"); ctx.Flash != "" {
		//log.Printf("DEBUG RenderHtml:GetCookie flash: %s", ctx.Flash)
		DelCookie(w, "flash")
	}

	ctx.CsrfField = csrf.TemplateField(r)

	if err := Templates.ExecuteTemplate(w, tmpl, ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func SetFlash(w http.ResponseWriter, mode, text string) {
	raw_msg := struct {
		Msg  string `json:"msg"`
		Text string `json:"text"`
	} {
		mode,
		text,
	}
	json_msg, _ := json.Marshal(raw_msg)
	SetCookie(w, "flash", string(json_msg))
}

func SetCookie(w http.ResponseWriter, name, value string) {
	//log.Printf("DEBUG SetCookie %s: '%s'", name, value)
	c := &http.Cookie{
		Name:     CookiePrefix + name,
		Value:    base64.URLEncoding.EncodeToString([]byte(value)),
		Path:     "/",
		MaxAge:   0,
		HttpOnly: true,
	}
	http.SetCookie(w, c)
}

func DelCookie(w http.ResponseWriter, name string) {
	c := &http.Cookie{
		Name:     CookiePrefix + name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	}
	http.SetCookie(w, c)
}

func GetCookie(r *http.Request, name string) string {
	c, err := r.Cookie(CookiePrefix + name)
	if err != nil {
		return ""
	}
	v, err := base64.URLEncoding.DecodeString(c.Value)
	if err == nil {
		value := string(v)
		//log.Printf("DEBUG GetCookie %s: '%s'", name, value)
		return value
	}
	return ""
}

func OpenDB(logmode bool) *gorm.DB {
	DB_Mutex.Lock()

	if Database == nil {
		db, err := gorm.Open(DB_Type, DB_ConnStr)
		if err != nil {
			log.Printf("FATAL OpenDB %s: %s", DB_Connect, err)
			os.Exit(1)
		}
		Database = db
	}
	Database.LogMode(logmode)

	return Database
}

func CloseDB() {
	if Database != nil {
		Database.Close()
		Database = nil
	}

	DB_Mutex.Unlock()
}
