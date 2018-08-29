package database

import (
	"log"
	"strconv"
	"strings"

	"github.com/coreos/bbolt"
)

var (
	highestIDBucket = []byte("highest_id")
)

type Database bolt.DB

func NewDatabase() *Database {
	db, err := bolt.Open("tumblr.db", 0644, nil)
	if err != nil {
		log.Panic(err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(highestIDBucket)
		if err != nil {
			return err
		}

		// Migrate existing entries over to the new blog-identifier format
		// TODO: remove this
		b.ForEach(func(k, v []byte) error {
			sk := string(k)

			if !strings.ContainsRune(sk, '.') {
				b.Put([]byte(sk+".tumblr.com"), v)
				b.Delete(k)
			}

			return nil
		})

		return nil
	})
	if err != nil {
		log.Panic(err)
	}

	return (*Database)(db)
}

func (s *Database) Close() {
	err := s.get().Close()
	if err != nil {
		log.Panic(err)
	}
}

func (s *Database) GetHighestID(blogName string) int64 {
	var highestID int64

	err := s.get().View(func(tx *bolt.Tx) error {
		b := tx.Bucket(highestIDBucket).Get([]byte(blogName))
		if b == nil {
			return nil
		}

		var err error
		highestID, err = strconv.ParseInt(string(b), 10, 64)
		return err
	})
	if err != nil {
		log.Panic(err)
	}

	return highestID
}

func (s *Database) SetHighestID(blogName string, highestID int64) {
	err := s.get().Update(func(tx *bolt.Tx) error {
		s := strconv.FormatInt(highestID, 10)
		return tx.Bucket(highestIDBucket).Put([]byte(blogName), []byte(s))
	})
	if err != nil {
		log.Panic(err)
	}
}

func (s *Database) get() *bolt.DB {
	return (*bolt.DB)(s)
}
