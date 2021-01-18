package boltdb

import (
	"bytes"
	"encoding/gob"
	"os"
	"strings"
	"time"

	"github.com/nextiva/nextkala/job"

	"github.com/boltdb/bolt"
	log "github.com/sirupsen/logrus"
)

var (
	jobBucket    = []byte("jobs")
	jobRunBucket = []byte("job_runs")
)

func GetBoltDB(path string) *BoltJobDB {
	if path != "" && !strings.HasSuffix(path, "/") {
		path += "/"
	}
	path += "jobdb.db"
	var perms os.FileMode = 0600
	database, err := bolt.Open(path, perms, &bolt.Options{Timeout: time.Second * 10}) //nolint:gomnd
	if err != nil {
		log.Fatal(err)
	}
	return &BoltJobDB{
		path:   path,
		dbConn: database,
	}
}

type BoltJobDB struct {
	dbConn *bolt.DB
	path   string
}

func (db *BoltJobDB) Close() error {
	return db.dbConn.Close()
}

func (db *BoltJobDB) GetAll() ([]*job.Job, error) {
	allJobs := []*job.Job{}

	err := db.dbConn.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(jobBucket)
		if err != nil {
			return err
		}

		err = bucket.ForEach(func(k, v []byte) error {
			buffer := bytes.NewBuffer(v)
			dec := gob.NewDecoder(buffer)
			j := new(job.Job)
			err = dec.Decode(j)

			if err != nil {
				return err
			}

			err = j.InitDelayDuration(false)

			if err != nil {
				return err
			}

			allJobs = append(allJobs, j)

			return nil
		})

		return err
	})

	return allJobs, err
}

func (db *BoltJobDB) Get(id string) (*job.Job, error) {
	j := new(job.Job)

	err := db.dbConn.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(jobBucket)

		v := b.Get([]byte(id))
		if v == nil {
			return job.ErrJobNotFound(id)
		}

		buf := bytes.NewBuffer(v)
		err := gob.NewDecoder(buf).Decode(j)

		return err
	})
	if err != nil {
		return nil, err
	}

	j.Id = id
	return j, nil
}

func (db *BoltJobDB) Delete(id string) error {
	err := db.dbConn.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(jobBucket)
		return bucket.Delete([]byte(id))
	})
	return err
}

func (db *BoltJobDB) Save(j *job.Job) error {
	err := db.dbConn.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(jobBucket)
		if err != nil {
			return err
		}

		buffer := new(bytes.Buffer)
		enc := gob.NewEncoder(buffer)
		err = enc.Encode(j)
		if err != nil {
			return err
		}

		err = bucket.Put([]byte(j.Id), buffer.Bytes())
		if err != nil {
			return err
		}
		return nil
	})
	return err
}

// SaveRun persists a Job Run.
func (db *BoltJobDB) SaveRun(run *job.JobStat) error {
	err := db.dbConn.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(jobRunBucket)
		if err != nil {
			return err
		}

		buffer := new(bytes.Buffer)
		enc := gob.NewEncoder(buffer)
		err = enc.Encode(run)
		if err != nil {
			return err
		}

		err = bucket.Put([]byte(run.Id), buffer.Bytes())
		if err != nil {
			return err
		}
		return nil
	})
	return err
}

// GetAllRuns returns all persisted runs for a job.
func (db *BoltJobDB) GetAllRuns(jobID string) ([]*job.JobStat, error) {
	allRuns := make([]*job.JobStat, 0)

	err := db.dbConn.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(jobRunBucket)
		if err != nil {
			return err
		}

		err = bucket.ForEach(func(k, v []byte) error {
			buffer := bytes.NewBuffer(v)
			dec := gob.NewDecoder(buffer)
			jobStats := new(job.JobStat)

			err = dec.Decode(jobStats)
			if err != nil {
				return err
			}

			allRuns = append(allRuns, jobStats)

			return nil
		})

		return err
	})

	return allRuns, err
}

func (db *BoltJobDB) UpdateRun(jobRun *job.JobStat) error {
	jobStat, err := db.GetRun(jobRun.Id)
	if err != nil {
		return err
	}
	jobStat.Status = jobRun.Status
	return db.SaveRun(jobStat)
}

// GetRun returns a persisted job run.
func (db *BoltJobDB) GetRun(id string) (*job.JobStat, error) {
	run := new(job.JobStat)

	err := db.dbConn.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(jobRunBucket)

		v := b.Get([]byte(id))
		if v == nil {
			return job.ErrJobNotFound(id)
		}

		buf := bytes.NewBuffer(v)
		err := gob.NewDecoder(buf).Decode(run)

		return err
	})
	if err != nil {
		return nil, err
	}

	run.Id = id
	return run, nil
}

func (db *BoltJobDB) DeleteRun(id string) error {
	err := db.dbConn.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(jobRunBucket)
		return bucket.Delete([]byte(id))
	})
	return err
}

func (db *BoltJobDB) ClearExpiredRuns() error {
	return nil
}
