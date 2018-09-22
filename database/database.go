package database

import (
	"strconv"

	"go.etcd.io/bbolt"
)

var (
	highestIDBucket = []byte("highest_id")
)

type Database bbolt.DB

func NewDatabase() (*Database, error) {
	db, err := bbolt.Open("tumblr.db", 0644, nil)
	if err != nil {
		return nil, err
	}

	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(highestIDBucket)
		return err
	})
	if err != nil {
		return nil, err
	}

	return (*Database)(db), nil
}

func (s *Database) Close() error {
	return s.get().Close()
}

func (s *Database) GetHighestID(blogName string) (int64, error) {
	var highestID int64

	err := s.get().View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(highestIDBucket).Get([]byte(blogName))
		if b == nil {
			return nil
		}

		var err error
		highestID, err = strconv.ParseInt(string(b), 10, 64)
		return err
	})
	if err != nil {
		return 0, err
	}

	return highestID, nil
}

func (s *Database) SetHighestID(blogName string, highestID int64) error {
	return s.get().Update(func(tx *bbolt.Tx) error {
		s := strconv.FormatInt(highestID, 10)
		return tx.Bucket(highestIDBucket).Put([]byte(blogName), []byte(s))
	})
}

func (s *Database) get() *bbolt.DB {
	return (*bbolt.DB)(s)
}
