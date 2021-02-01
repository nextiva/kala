package job

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"strings"

	"github.com/mattn/go-shellwords"
	log "github.com/sirupsen/logrus"
)

type JobRunner struct {
	job  *Job
	meta Metadata

	numberOfAttempts uint
	currentRetries   uint
	currentStat      *JobStat
}

var (
	ErrJobDisabled       = errors.New("Job cannot run, as it is disabled")
	ErrCmdIsEmpty        = errors.New("Job Command is empty.")
	ErrJobTypeInvalid    = errors.New("Job Type is not valid.")
	ErrInvalidDelimiters = errors.New("Job has invalid templating delimiters.")
)

// Run calls the appropriate run function, collects metadata around the success
// or failure of the Job's execution, and schedules the next run.
func (j *JobRunner) Run(cache JobCache) (*JobStat, Metadata, error) {
	j.job.lock.RLock()
	defer j.job.lock.RUnlock()

	j.meta.LastAttemptedRun = j.job.clk.Time().Now()

	if j.job.Disabled {
		log.Infof("Job %s tried to run, but exited early because its disabled.", j.job.Name)
		return nil, j.meta, ErrJobDisabled
	}

	log.Infof("Job %s:%s started.", j.job.Name, j.job.Id)

	j.runSetup()

	var out string
	for {
		var err error
		switch {
		case j.job.succeedInstantly:
			out = "Job succeeded instantly for test purposes."
		case j.job.JobType == LocalJob:
			out, err = j.LocalRun()
		case j.job.JobType == RemoteJob:
			j.currentStat.Status = Status.Started
			err = cache.SaveRun(j.currentStat)
			if err != nil {
				log.Errorf("Error saving initial job status: %v", err)
			}
			out, err = j.RemoteRun()
		default:
			err = ErrJobTypeInvalid
		}

		j.currentStat.Output = out

		if err != nil {
			// Log Error in Metadata
			log.Errorf("Error running job %s with execution id %s: %v", j.currentStat.JobId, j.currentStat.Id,
				err)

			mailErr := NotifyOfJobFailure(j.job, j.currentStat)
			if mailErr != nil {
				log.Errorln("Error notifying of job failure:", mailErr)
			}

			j.meta.ErrorCount++
			j.meta.LastError = j.job.clk.Time().Now()

			// Handle retrying
			if j.shouldRetry() {
				j.currentRetries--
				continue
			}

			j.collectStats(Status.Failed)
			j.meta.NumberOfFinishedRuns++

			// TODO: Wrap error into something better.
			return j.currentStat, j.meta, err
		} else {
			break
		}
	}

	log.Infof("Job %s:%s finished.", j.job.Name, j.job.Id)
	log.Debugf("Job %s:%s output: %s", j.job.Name, j.job.Id, out)

	j.meta.SuccessCount++
	j.meta.NumberOfFinishedRuns++
	j.meta.LastSuccess = j.job.clk.Time().Now()

	if j.job.JobType == RemoteJob {
		// Stats have already been saved.
		j.currentStat = nil
	} else {
		j.collectStats(Status.Success)
	}

	// Run Dependent Jobs
	if len(j.job.DependentJobs) != 0 {
		for _, id := range j.job.DependentJobs {
			newJob, err := cache.Get(id)
			if err != nil {
				log.Errorf("Error retrieving dependent job with id of %s", id)
			} else {
				newJob.Run(cache)
			}
		}
	}

	return j.currentStat, j.meta, nil
}

// LocalRun executes the Job's local shell command
func (j *JobRunner) LocalRun() (string, error) {
	return j.runCmd()
}

// RemoteRun sends a http request, and checks if the response is valid in time,
func (j *JobRunner) RemoteRun() (string, error) {
	// Calculate a response timeout
	timeout := j.job.ResponseTimeout()

	ctx := context.Background()
	if timeout > 0 {
		var cncl func()
		ctx, cncl = context.WithTimeout(ctx, timeout)
		defer cncl()
	}
	// Get the actual url and body we're going to be using,
	// including any necessary templating.
	url, err := j.job.TryTemplatize(j.job.RemoteProperties.Url)
	if err != nil {
		return "", fmt.Errorf("Error templatizing url: %v", err)
	}
	body, err := j.job.TryTemplatize(j.job.RemoteProperties.Body)
	if err != nil {
		return "", fmt.Errorf("Error templatizing body: %v", err)
	}

	// Normalize the method passed by the user
	method := strings.ToUpper(j.job.RemoteProperties.Method)
	bodyBuffer := bytes.NewBufferString(body)
	req, err := http.NewRequest(method, url, bodyBuffer)
	if err != nil {
		return "", err
	}

	token, err := GetJobToken(ctx)
	if err != nil {
		return "", err
	}
	if Oauth2Config != nil && username != "" && password != "" {
		authToken, err := Oauth2Config.PasswordCredentialsToken(ctx, username, password)
		if err != nil {
			log.Errorf("Unable to obtain token for user %s: %v", username, err)
			return "", err
		}
		if authToken.AccessToken == "" {
			log.Errorf("Access token not returned for usr %s", username)
			return "", errors.New("Unable to obtain access token for user" + username)
		}
		token = authToken.AccessToken
	}

	// Set default or user's passed headers
	j.setHeaders(req, token)

	// Do the request
	res, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	// Check if we got any of the status codes the user asked for
	if j.checkExpected(res.StatusCode) {
		return string(b), nil
	} else {
		return "", errors.New(res.Status + string(b))
	}
}

func initShParser() *shellwords.Parser {
	shParser := shellwords.NewParser()
	shParser.ParseEnv = true
	shParser.ParseBacktick = true
	return shParser
}

func (j *JobRunner) runCmd() (string, error) {
	j.numberOfAttempts++

	// Get the actual command we're going to be running,
	// including any necessary templating.
	cmdText, err := j.job.TryTemplatize(j.job.Command)
	if err != nil {
		return "", fmt.Errorf("Error templatizing command: %v", err)
	}

	// Execute command
	shParser := initShParser()
	args, err := shParser.Parse(cmdText)
	if err != nil {
		return "", err
	}
	if len(args) == 0 {
		return "", ErrCmdIsEmpty
	}

	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec // That's the job description
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func (j *JobRunner) shouldRetry() bool {
	// Check number of retries left
	if j.currentRetries == 0 {
		return false
	}

	// Check Epsilon
	if j.job.Epsilon != "" && j.job.Schedule != "" {
		if !j.job.epsilonDuration.IsZero() {
			timeSinceStart := j.job.clk.Time().Now().Sub(j.job.NextRunAt)
			timeLeftToRetry := j.job.epsilonDuration.RelativeTo(j.job.clk.Time().Now()) - timeSinceStart
			if timeLeftToRetry < 0 {
				return false
			}
		}
	}

	return true
}

func (j *JobRunner) runSetup() {
	// Setup Job Stat
	j.currentStat = NewJobStat(j.job.Id)
	j.currentStat.Status = Status.Success

	// Init retries
	j.currentRetries = j.job.Retries
}

func (j *JobRunner) collectStats(status JobStatus) {
	j.currentStat.ExecutionDuration = j.job.clk.Time().Now().Sub(j.currentStat.RanAt)
	j.currentStat.Status = status
	j.currentStat.NumberOfRetries = j.job.Retries - j.currentRetries
}

func (j *JobRunner) checkExpected(statusCode int) bool {
	// If no expected response codes passed, add 200 status code as expected
	if len(j.job.RemoteProperties.ExpectedResponseCodes) == 0 {
		j.job.RemoteProperties.ExpectedResponseCodes = append(j.job.RemoteProperties.ExpectedResponseCodes, 200)
	}
	for _, expected := range j.job.RemoteProperties.ExpectedResponseCodes {
		if expected == statusCode {
			return true
		}
	}

	return false
}

// setHeaders sets default and user specific headers to the http request
func (j *JobRunner) setHeaders(req *http.Request, token string) {
	j.job.SetHeaders(req, token)
	if j.currentStat != nil {
		j.job.RemoteProperties.Headers.Set("NextKala-JobId", j.job.Id)
		j.job.RemoteProperties.Headers.Set("NextKala-RunId", j.currentStat.Id)
	}
}
