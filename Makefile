#
#
#

SRC := $(wildcard *.go)
TPL := $(wildcard templates/*.html)
LOC := $(wildcard locales/*.json)

postfix-go: $(SRC) $(TPL) $(LOC)
	ctags *.go
	go build

tags: $(SRC) $(TPL) $(LOC)
	ctags *.go

run: postfix-go
	./postfix-go -v

css:
	sed -i -e "s#DataTables.*/images#../img#g" static/css/datatables.css
	sed -i -e "s#DataTables.*/images#../img#g" static/css/datatables.min.css

clean:
	rm -f tags postfix-go.sql

real-clean: clean
	rm -f postfix-go

fresh: clean postfix-go
	./postfix-go -v

update:
	go get -u golang.org/x/crypto/bcrypt
	go get -u github.com/spf13/viper
	go get -u github.com/julienschmidt/httprouter
	go get -u github.com/gorilla/securecookie
	go get -u github.com/gorilla/csrf
	go get -u github.com/jinzhu/gorm
	go get -u github.com/jinzhu/gorm/dialects/sqlite
	go get -u github.com/go-sql-driver/mysql
	go get -u github.com/nicksnyder/go-i18n/i18n

