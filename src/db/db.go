package db

import (
	"bufio"
	"compress/gzip"
	"database/sql"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
	"github.com/schollz/cowyo2/src/utils"
	"github.com/schollz/sqlite3dump"
)

type FileSystem struct {
	name string
	db   *sql.DB
	sync.RWMutex
}

// File is the basic unit that is saved
type File struct {
	ID       string
	Slug     string
	Created  time.Time
	Modified time.Time
	Data     string
}

// New will initialize a filesystem
func New(name string) (fs *FileSystem, err error) {
	fs = new(FileSystem)
	if name == "" {
		err = errors.New("database must have name")
		return
	}
	fs.name = name

	// if read-only, make sure the database exists
	fs.db, err = sql.Open("sqlite3", ":memory:")
	if err != nil {
		return
	}
	err = fs.initializeDB()
	if err != nil {
		err = errors.Wrap(err, "could not initialize")
		return
	}
	return
}

func (fs *FileSystem) initializeDB() (err error) {
	if _, errHaveSQL := os.Stat(fs.name + ".sql.gz"); errHaveSQL == nil {
		fi, err := os.Open(fs.name + ".sql.gz")
		if err != nil {
			return err
		}
		defer fi.Close()

		fz, err := gzip.NewReader(fi)
		if err != nil {
			return err
		}
		defer fz.Close()

		s, err := ioutil.ReadAll(fz)
		if err != nil {
			return err
		}
		_, err = fs.db.Exec(string(s))
		return err
	}
	sqlStmt := `CREATE TABLE 
		fs (
			id TEXT NOT NULL PRIMARY KEY, 
			slug TEXT,
			created TIMESTAMP,
			modified TIMESTAMP,
			data TEXT
		);`
	_, err = fs.db.Exec(sqlStmt)
	if err != nil {
		err = errors.Wrap(err, "creating table")
		return
	}

	sqlStmt = `CREATE VIRTUAL TABLE 
		fts USING fts5 (id,data);`
	_, err = fs.db.Exec(sqlStmt)
	if err != nil {
		err = errors.Wrap(err, "creating virtual table")
	}
	return
}

// DumpSQL will dump the SQL as text to filename.sql
func (fs *FileSystem) DumpSQL() (err error) {
	fs.Lock()
	defer fs.Unlock()
	fi, err := os.Create(fs.name + ".sql.gz")
	if err != nil {
		return
	}
	gf := gzip.NewWriter(fi)
	fw := bufio.NewWriter(gf)
	err = sqlite3dump.DumpDB(fs.db, fw)
	fw.Flush()
	gf.Close()
	fi.Close()
	return
}

// NewFile returns a new file
func (fs *FileSystem) NewFile(slug, data string) (f File) {
	f = File{
		ID:       utils.UUID(),
		Slug:     slug,
		Created:  time.Now(),
		Modified: time.Now(),
		Data:     data,
	}
	return
}

// Save a file to the file system. Will insert or ignore, and then update.
func (fs *FileSystem) Save(f File) (err error) {
	fs.Lock()
	defer fs.Unlock()

	tx, err := fs.db.Begin()
	if err != nil {
		return errors.Wrap(err, "begin Save")
	}

	stmt, err := tx.Prepare(`
	INSERT OR IGNORE INTO
		fs
	(
		id, 
		slug,
		created,
		modified
	) 
		values 	
	(
		?, 
		?,
		?,
		?
	)`)
	if err != nil {
		return errors.Wrap(err, "stmt Save")
	}

	_, err = stmt.Exec(
		f.ID,
		f.Slug,
		f.Created,
		time.Now(),
	)
	if err != nil {
		return errors.Wrap(err, "exec Save")
	}
	defer stmt.Close()
	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "commit Save")
	}

	// if it was ignored
	tx2, err := fs.db.Begin()
	if err != nil {
		return errors.Wrap(err, "begin Save")
	}
	stmt2, err := tx2.Prepare(`
	UPDATE fs SET 
		slug = ?,
		modified = ?
	WHERE
		id = ?
	`)
	if err != nil {
		return errors.Wrap(err, "stmt update")
	}
	defer stmt2.Close()

	_, err = stmt2.Exec(
		f.Slug,
		time.Now(),
		f.ID,
	)
	if err != nil {
		return errors.Wrap(err, "exec update")
	}
	err = tx2.Commit()
	if err != nil {
		return errors.Wrap(err, "commit update")
	}

	// check if exists in fts
	sqlStmt := "INSERT INTO fts(data,id) VALUES (?,?)"
	var ftsHasID bool
	ftsHasID, err = fs.idExists(f.ID)
	if err != nil {
		return errors.Wrap(err, "doesExist")
	}
	if ftsHasID {
		sqlStmt = "UPDATE fts SET data=? WHERE id=?"
	}

	// update the index
	tx3, err := fs.db.Begin()
	if err != nil {
		return errors.Wrap(err, "begin virtual Save")
	}
	stmt3, err := tx3.Prepare(sqlStmt)
	if err != nil {
		return errors.Wrap(err, "stmt virtual update")
	}
	defer stmt3.Close()

	_, err = stmt3.Exec(
		f.Data,
		f.ID,
	)
	if err != nil {
		return errors.Wrap(err, "exec virtual update")
	}
	err = tx3.Commit()
	if err != nil {
		return errors.Wrap(err, "commit virtual update")
	}
	return

}

// Close will make sure that the lock file is closed
func (fs *FileSystem) Close() (err error) {
	return fs.db.Close()
}

// Len returns how many things
func (fs *FileSystem) Len() (l int, err error) {
	fs.Lock()
	defer fs.Unlock()

	// prepare statement
	query := "SELECT COUNT(id) FROM FS"
	stmt, err := fs.db.Prepare(query)
	if err != nil {
		err = errors.Wrap(err, "preparing query: "+query)
		return
	}

	defer stmt.Close()
	rows, err := stmt.Query()
	if err != nil {
		err = errors.Wrap(err, query)
		return
	}

	// loop through rows
	defer rows.Close()
	for rows.Next() {
		err = rows.Scan(&l)
		if err != nil {
			err = errors.Wrap(err, "getRows")
			return
		}
	}
	err = rows.Err()
	if err != nil {
		err = errors.Wrap(err, "getRows")
	}
	return
}

// Get returns the info from a file
func (fs *FileSystem) Get(id string) (files []File, err error) {
	fs.Lock()
	defer fs.Unlock()

	files, err = fs.getAllFromPreparedQuery(`
		SELECT fs.id,fs.slug,fs.created,fs.modified,fts.data FROM fs INNER JOIN fts ON fs.id=fts.id WHERE fs.id = ? ORDER BY modified DESC`, id)
	if err != nil {
		err = errors.Wrap(err, "Stat")
		return
	}
	if len(files) > 0 {
		return
	}

	files, err = fs.getAllFromPreparedQuery(`
		SELECT fs.id,fs.slug,fs.created,fs.modified,fts.data FROM fs INNER JOIN fts ON fs.id=fts.id WHERE fs.id IN (SELECT id FROM fs WHERE slug=?) ORDER BY modified DESC`, id)
	if err != nil {
		err = errors.Wrap(err, "get from slug")
		return
	}
	if len(files) > 0 {
		return
	}

	err = errors.New("no files with that slug or id")
	return
}

// LastModified get the last modified time
func (fs *FileSystem) LastModified() (lastModified time.Time, err error) {
	// prepare statement
	query := "SELECT modified FROM fs ORDER BY modified DESC LIMIT 1"
	stmt, err := fs.db.Prepare(query)
	if err != nil {
		err = errors.Wrap(err, "preparing query: "+query)
		return
	}

	defer stmt.Close()
	rows, err := stmt.Query()
	if err != nil {
		err = errors.Wrap(err, query)
		return
	}

	// loop through rows
	defer rows.Close()
	for rows.Next() {
		err = rows.Scan(&lastModified)
		if err != nil {
			err = errors.Wrap(err, "getRows")
			return
		}
	}
	err = rows.Err()
	if err != nil {
		err = errors.Wrap(err, "getRows")
	}
	return
}

// Find returns the info from a file
func (fs *FileSystem) Find(text string) (files []File, err error) {
	fs.Lock()
	defer fs.Unlock()

	files, err = fs.getAllFromPreparedQuery(`
		SELECT fs.id,fs.slug,fs.created,fs.modified,fts.data FROM fs INNER JOIN fts ON fs.id=fts.id WHERE fs.id IN (SELECT id FROM fts WHERE data MATCH ?) ORDER BY modified DESC`, text)
	log.Printf(`SELECT fs.id,fs.slug,fs.created,fs.modified,fts.data FROM fs INNER JOIN fts ON fs.id=fts.id WHERE fs.id IN (SELECT id FROM fts WHERE data MATCH '%s') ORDER BY modified DESC`, text)
	return
}

// Exists returns whether specified ID exists exists
func (fs *FileSystem) idExists(id string) (exists bool, err error) {
	files, err := fs.getAllFromPreparedQuerySingleString(`
		SELECT id FROM fts WHERE id = ?`, id)
	if err != nil {
		err = errors.Wrap(err, "Exists")
	}
	if len(files) > 0 {
		exists = true
	}
	return
}

// Exists returns whether specified id or slug exists
func (fs *FileSystem) Exists(id string) (exists bool, err error) {
	fs.Lock()
	defer fs.Unlock()

	files, err := fs.getAllFromPreparedQuerySingleString(`
		SELECT id FROM fs WHERE id = ?`, id)
	if err != nil {
		err = errors.Wrap(err, "Exists")
	}
	if len(files) > 0 {
		exists = true
		return
	}

	files, err = fs.getAllFromPreparedQuerySingleString(`
	SELECT id FROM fs WHERE slug = ?`, id)
	if err != nil {
		err = errors.Wrap(err, "Exists")
	}
	if len(files) > 0 {
		exists = true
	}

	return
}

func (fs *FileSystem) getAllFromPreparedQuery(query string, args ...interface{}) (files []File, err error) {
	// prepare statement
	stmt, err := fs.db.Prepare(query)
	if err != nil {
		err = errors.Wrap(err, "preparing query: "+query)
		return
	}

	defer stmt.Close()
	rows, err := stmt.Query(args...)
	if err != nil {
		err = errors.Wrap(err, query)
		return
	}

	// loop through rows
	defer rows.Close()
	files = []File{}
	for rows.Next() {
		var f File
		err = rows.Scan(
			&f.ID,
			&f.Slug,
			&f.Created,
			&f.Modified,
			&f.Data,
		)
		if err != nil {
			err = errors.Wrap(err, "getRows")
			return
		}
		files = append(files, f)
	}
	err = rows.Err()
	if err != nil {
		err = errors.Wrap(err, "getRows")
	}
	return
}

func (fs *FileSystem) getAllFromPreparedQuerySingleString(query string, args ...interface{}) (s []string, err error) {
	// prepare statement
	stmt, err := fs.db.Prepare(query)
	if err != nil {
		err = errors.Wrap(err, "preparing query: "+query)
		return
	}

	defer stmt.Close()
	rows, err := stmt.Query(args...)
	if err != nil {
		err = errors.Wrap(err, query)
		return
	}

	// loop through rows
	defer rows.Close()
	s = []string{}
	for rows.Next() {
		var stemp string
		err = rows.Scan(
			&stemp,
		)
		if err != nil {
			err = errors.Wrap(err, "getRows")
			return
		}
		s = append(s, stemp)
	}
	err = rows.Err()
	if err != nil {
		err = errors.Wrap(err, "getRows")
	}
	return
}
