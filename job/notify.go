package job

import (
	"fmt"

	"gopkg.in/mail.v2"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Mailer struct {
	host        string
	port        int
	userName    string
	password    string
	fromAddress string
	skipVerify  bool
}

var mailer *Mailer

func InitMailer() {
	viper.SetDefault("mailer.skipVerify", false)
	host := viper.GetString("mailer.host")
	port := viper.GetInt("mailer.port")
	mailUsername := viper.GetString("mailer.username")
	mailPassword := viper.GetString("mailer.password")
	fromAddress := viper.GetString("mailer.fromAddress")
	skipVerify := viper.GetBool("mailer.skipVerify")
	if host == "" || port == 0 {
		return
	}
	if fromAddress == "" {
		log.Warnf("No mailer fromAddress configured. Cannot send mail")
		return
	}
	mailer = &Mailer{host: host,
		port:        port,
		userName:    mailUsername,
		password:    mailPassword,
		fromAddress: fromAddress,
		skipVerify:  skipVerify,
	}
}

func Notify(toAddress string, subject string, message string) error {
	if mailer != nil {
		msg := mail.NewMessage()
		msg.SetHeader("From", mailer.fromAddress)
		msg.SetHeader("To", toAddress)
		msg.SetHeader("Subject", subject)
		msg.SetBody("text/plain", message)

		dialer := mail.NewDialer(mailer.host, mailer.port, mailer.userName, mailer.password)
		if !mailer.skipVerify {
			dialer.StartTLSPolicy = mail.MandatoryStartTLS
		}
		err := dialer.DialAndSend(msg)
		if err != nil {
			log.Errorf("Unable to send email due to %s", err)
		}
		return err
	}
	return nil
}

func NotifyOfJobFailure(j *Job, run *JobStat) error {
	subject := fmt.Sprintf("Job %s Failed", j.Name)

	url := fmt.Sprintf("<a href=\"http://kalaurl/webui/job/execution/%s\">Job Run Link</a>", run.Id)
	msg := fmt.Sprintf("Hi!  Please be advised that your job failed.  Details: %+v Job Run: %s", run, url)

	err := Notify(j.Owner, subject, msg)
	return err
}
