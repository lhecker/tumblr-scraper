package database

import (
	"strconv"

	"go.etcd.io/bbolt"
)

var (
	stateBucket     = []byte("state")
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

func (s *Database) GetCookies() (snapshot []byte, err error) {
	err = s.get().Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(stateBucket)
		if err != nil {
			return err
		}

		snapshot = b.Get([]byte("cookies"))
		return nil
	})
	return
}

func (s *Database) SaveCookies(snapshot []byte) error {
	return s.get().Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(stateBucket)
		if err != nil {
			return err
		}

		return b.Put([]byte("cookies"), snapshot)
	})
}

func (s *Database) GetHighestID(blogName string) (int64, error) {
	var highestID int64

	err := s.get().Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(highestIDBucket)
		if err != nil {
			return err
		}

		data := b.Get([]byte(blogName))
		if len(data) == 0 {
			return nil
		}

		highestID, err = strconv.ParseInt(string(data), 10, 64)
		return err
	})
	if err != nil {
		return 0, err
	}

	return highestID, nil
}

func (s *Database) SetHighestID(blogName string, highestID int64) error {
	return s.get().Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(highestIDBucket)
		if err != nil {
			return err
		}

		s := strconv.FormatInt(highestID, 10)
		return b.Put([]byte(blogName), []byte(s))
	})
}

func (s *Database) get() *bbolt.DB {
	return (*bbolt.DB)(s)
}
