package main
import (
	"fmt"
	"regexp"
	"strings"
)
func main() {
	validDomainRe := regexp.MustCompile(` + "`" + `^[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?)+$` + "`" + `)
	abpRe := regexp.MustCompile(` + "`" + `^\|\|(.+?)\^` + "`" + `)
	
	fmt.Println(validDomainRe.MatchString("google.com"))
}
