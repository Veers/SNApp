package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
	"time"

	"github.com/ricochet2200/go-disk-usage/du"
)

var KB = uint64(1024)

type SMTPConfiguration struct {
	server   string
	port     int
	from     string
	login    string
	password string
	reply    string
	to       []string
}

type DataVolume struct {
	VolumeGOOSLetter string
	VolumeUNIXPath   string
	VolumeFolders    []string
}

type Configuration struct {
	Volumes  []DataVolume
	MailList []string
	Params   Params
}

type Params struct {
	Percent                   int
	DateFormat_date           string
	DateFormat_time           string
	ThomeOutputFormat         string
	ThomeOutputFormat_success string
	ThomeOutputFormat_error   string
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func getThomeValues(path string, params Params) string {
	thomeVars := make(map[string]interface{})

	thomeVars["ThomePercent"] = "percent"
	thomeVars["ThomeStatus"] = "status"

	// parse the template
	tmpl, _ := template.ParseFiles("templates/thomeinfo.tmpl")
	var tpl bytes.Buffer

	usage := du.NewDiskUsage(path)
	thomePercent := usage.Usage() * 100
	// fmt.Sprintf("%F %%", usage.Usage()*100)
	// thomePercent := 23.05643
	if int(thomePercent) >= params.Percent {
		thomeVars["ThomeStatus"] = params.ThomeOutputFormat_error
	} else {
		thomeVars["ThomeStatus"] = params.ThomeOutputFormat_success
	}

	if runtime.GOOS == "windows" {
		thomeVars["ThomeName"] = path
	} else {
		thomeVars["ThomeName"] = filepath.FromSlash(strings.Split(path, "/")[2])
	}

	thomeVars["ThomePercent"] = fmt.Sprintf("%.2f %%", thomePercent)
	tmpl.Execute(&tpl, thomeVars)
	return tpl.String()
}

// func sendEmails() {
// 	smptConfigFile, _ := os.Open("smtp_conf.json")
// 	defer smptConfigFile.Close()
// 	smtpDecoder := json.NewDecoder(smptConfigFile)
// 	smtpConfiguration := SMTPConfiguration()
// 	err := smtpDecoder.Decode(&smtpConfiguration)
// 	if err != nil {
// 		fmt.Println("error:", err)
// 	}

// 	msg := "Hello geeks!!!"
// 	body := []byte(msg)
// 	auth := smtp.PlainAuth("", smtpConfiguration.from, smtpConfiguration.password, smtpConfiguration.host)
// 	err_smtp := smtp.SendMail(
// 		smtpConfiguration.server+":"+smtpConfiguration.port,
// 		auth,
// 		smtpConfiguration.from,
// 		smtpConfiguration.to,
// 		body)
// 	if err_smtp != nil {
// 		fmt.Println(err_smtp)
// 		os.Exit(1)
// 	}

// 	fmt.Println("Successfully sent mail to all user in toList")
// }

func main() {

	startDate := time.Now()

	// read config start
	file, _ := os.Open("conf.json")
	defer file.Close()
	decoder := json.NewDecoder(file)
	configuration := Configuration{}
	err := decoder.Decode(&configuration)
	if err != nil {
		fmt.Println("error:", err)
	}
	fmt.Println(configuration.Volumes) // output: [UserA, UserB]
	// read config start

	// check disk (dir) size starts
	usage := du.NewDiskUsage("/")
	fmt.Println("Free:", usage.Free()/(KB*KB*KB))
	fmt.Println("Available:", usage.Available()/(KB*KB*KB))
	fmt.Println("Size:", usage.Size()/(KB*KB*KB))
	fmt.Println("Used:", usage.Used()/(KB*KB*KB))
	fmt.Println("Usage:", usage.Usage()*100, "%")
	// check disk (dir) size starts

	time.Sleep(2 * time.Second)

	// set := make(map[string]bool)

	thomeInfo := make([]string, len(configuration.Volumes))
	var thome string
	for _, s := range configuration.Volumes {
		if runtime.GOOS == "windows" {
			// fmt.Println("Hello from Windows")
			thome = filepath.FromSlash(s.VolumeGOOSLetter + ":")
		} else {
			thome = filepath.FromSlash(s.VolumeUNIXPath)
		}
		thomeInfo = append(thomeInfo, strings.TrimSpace(getThomeValues(thome, configuration.Params)))
	}
	// sendEmails()

	thomeResText := strings.Join(thomeInfo, "\r\n")

	time.Sleep(2 * time.Second)

	endDate := time.Now()

	vars := make(map[string]interface{})
	vars["Time"] = startDate.Format(configuration.Params.DateFormat_time)
	vars["Date"] = startDate.Format(configuration.Params.DateFormat_date)
	vars["DateEnd"] = endDate.Format(configuration.Params.DateFormat_date)
	vars["TimeEnd"] = endDate.Format(configuration.Params.DateFormat_time)
	vars["ThomeInfo"] = strings.TrimSpace(thomeResText)
	vars["ProjectInfos"] = "TEST"
	// parse the template
	tmpl, _ := template.ParseFiles("templates/template.tmpl")
	// create a new file
	file1, _ := os.Create("greeting.txt")
	defer file1.Close()
	// apply the template to the vars map and write the result to file.
	tmpl.Execute(file1, vars)
}
