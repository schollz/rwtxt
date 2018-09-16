package db

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBasic(t *testing.T) {
	os.Remove("test.db")

	fs, err := New("test.db")
	assert.Nil(t, err)

	f := fs.NewFile("test1", "someslug", "some text")
	assert.Nil(t, err)
	err = fs.Save(f)
	assert.Nil(t, err)
	time.Sleep(1 * time.Second)
	err = fs.Save(f)
	assert.Nil(t, err)

	f2, err := fs.Get("test1")
	assert.Equal(t, f.Data, f2.Data)
	assert.Nil(t, err)
	assert.True(t, f2.Modified.Second()-f.Modified.Second() >= 1)

	exists, err := fs.Exists("doesn't exist")
	assert.Nil(t, err)
	assert.False(t, exists)
	exists, err = fs.Exists("test1")
	assert.Nil(t, err)
	assert.True(t, exists)

	err = fs.DumpSQL()
	assert.Nil(t, err)
}
