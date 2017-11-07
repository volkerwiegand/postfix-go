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
	Minified       string
	CsrfField      template.HTML
	StyleSheets    []string
	JavaScripts    []string
	PriButton      string
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
	Web_Token     string
	Def_Domain    string
	ProdMode      bool
	Verbose       bool
	prodTemplates *template.Template
	funcMap       template.FuncMap
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

	viper.SetDefault("Language",   "de")
	viper.SetDefault("DB_Type",    "sqlite3")
	viper.SetDefault("Web_Addr",   ":8000")
	viper.SetDefault("DB_Connect", "postfix-go.sql")
	viper.SetDefault("Web_Token",  "_Postfix_Dovecot_Golang_PureCSS_")	// 32 bytes
	viper.SetDefault("Def_Domain", "example.com")
	viper.SetDefault("ProdMode",   false)
	viper.SetDefault("Verbose",    false)

	if err := viper.ReadInConfig(); err != nil {
		log.Printf("FATAL ReadInConfig: %s", err)
		os.Exit(1)
	}

	Language   = viper.GetString("Language")
	DB_Type    = viper.GetString("DB_Type")
	DB_Connect = viper.GetString("DB_Connect")
	Web_Addr   = viper.GetString("Web_Addr")
	Web_Token  = viper.GetString("Web_Token")
	Def_Domain = viper.GetString("Def_Domain")
	ProdMode   = viper.GetBool("ProdMode")
	Verbose    = viper.GetBool("Verbose")

	if DB_Type == "mysql" {
		DB_ConnStr = DB_Connect + "?charset=utf8&parseTime=True&loc=Local"
	} else {
		DB_ConnStr = DB_Connect
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
	DomainInit()
	AliasInit()
	AddressInit()

	//
	// Initialize templates and function map
	//
	funcMap = template.FuncMap{
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
	if ProdMode {
		prodTemplates = template.Must(template.New("").Funcs(funcMap).ParseGlob("templates/*"))
	}

	//
	// Setup the web server and router
	//
	r := httprouter.New()

	r.ServeFiles("/static/*filepath", http.Dir("./static"))

	r.GET("/",                   HomeIndex)
	r.GET("/login",              LoginLoginGet)
	r.POST("/login",             LoginLoginPost)
	r.GET("/logout",             LoginLogout)
	r.GET("/domain",             DomainCreate)
	r.GET("/domain/:id",         DomainEdit)
	r.POST("/domain/:id",        DomainUpdate)
	r.GET("/domain/:id/delete",  DomainDelete)
	r.GET("/address",            AddressCreate)
	r.GET("/address/:id",        AddressEdit)
	r.POST("/address/:id",       AddressUpdate)
	r.GET("/address/:id/print",  AddressPrint)
	r.GET("/address/:id/delete", AddressDelete)
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
	var runTemplates *template.Template

	ctx.Language = Language

	if ctx.Flash = GetCookie(r, "flash"); ctx.Flash != "" {
		//log.Printf("INFO  RenderHtml:GetCookie flash: %s", ctx.Flash)
		DelCookie(w, "flash")
	}

	ctx.CsrfField = csrf.TemplateField(r)

	if ProdMode {
		ctx.Minified = ".min"
		runTemplates = prodTemplates
	} else {
		langfile := fmt.Sprintf("locales/%s.all.json", Language)
		if err := i18n.LoadTranslationFile(langfile); err != nil {
			log.Printf("FATAL LoadTranslationFile %s: %s", langfile, err)
			os.Exit(1)
		}

		ctx.Minified = ""
		runTemplates = template.Must(template.New("").Funcs(funcMap).ParseGlob("templates/*"))
	}

	if err := runTemplates.ExecuteTemplate(w, tmpl, ctx); err != nil {
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
	//log.Printf("INFO  SetCookie %s: '%s'", name, value)
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
		//log.Printf("INFO  GetCookie %s: '%s'", name, value)
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
