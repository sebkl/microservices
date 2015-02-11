//package shareme offers an experimental google appengine example service which allows
// to share files based on a hash key which can be distributed as an URL. It demonstrates 
// the  use of the blobstore functionality in combination with the gotojs package for
// appengine integration.
//
// TODOS:
//	- Check valid period by ornjob or on certain calls.
//	- configurable file type and size limits on both, server and client side.
//	- store meta information such as name in datastore
//	- support multiple files
//	- add sento link (mailto:)
//	- add copy button which copies link to clipboard (!)
//	- general UI improvements
//	- consider authentication and authorization options per share.
//	- add thumbnail 50x50 in stats object.
//	- implement/use url shortener (together with datastore)
package shareme

import (
	. "github.com/sebkl/gotojs"
	"github.com/sebkl/gotojs/gae"
	"github.com/sebkl/microservices/gotojs/imgsrv"
	"log"
	"io"
	"appengine"
	"appengine/blobstore"
	"appengine/user"
	"time"
	"errors"
	"net/http"
	"fmt"
	"strings"
)

const (
	DefaultValidPeriod = 7 * 24 * 60 * 60 * 1000 //7days in msecs
	Msec = 1000 * 1000
	KeySessionPrefix = "key:"
	InterfaceName = "Shareme"
)

type Share struct {
	Key string `json:"key,omitempty"`
	ValidPeriod int64 `json:"valid_period,omitempty"` //msec
	Size int64 `json:"size,omitempty"`
	Name string `json:"name,omitempty"`
	Created int64 `json:"created,omitempty"` //msec unix
	MimeType string `json:"mimetype,omitempty"`
	Error string `json:"error,omitempty"`
	Url string `json:"url,omitempty"`
}

func (s *Share) IsError() bool {
	return len(s.Error) > 0
}

func newShare(gae *gae.GAEContext, bi *blobstore.BlobInfo) Share {
	return Share{
		Key: string(bi.BlobKey),
		ValidPeriod: DefaultValidPeriod - int64(time.Now().Sub(bi.CreationTime).Nanoseconds() / Msec),
		Size: bi.Size,
		MimeType: bi.ContentType,
		Created: int64(bi.CreationTime.UnixNano() / Msec),
		Url: fmt.Sprintf("%s/%s/%s/%s",gae.HTTPContext.Frontend.BaseUrl(),InterfaceName,"Get",string(bi.BlobKey)),
	}
}

func noShare(err error) Share {
	return Share{
		Error: err.Error(),
		Size: 0,
		ValidPeriod: 0,
	}
}

type BlobBinary struct {
	io.Reader
	mimeType string
}

func (b *BlobBinary) MimeType() string {
	return b.mimeType
}

func (b *BlobBinary) Close() error {
	return nil
}

type SharemeService struct {
}

func (s *SharemeService) Get(c* gae.GAEContext, key string) interface{} { // Return either Share in case of Error or Binary.
	k := appengine.BlobKey(key)
	bi,err := blobstore.Stat(c,k)
	if err != nil {
		c.HTTPContext.ReturnStatus = http.StatusNotFound
		return noShare(err)
	}

	reader :=blobstore.NewReader(c,k)
	//TODO: set Content-Disposition.
	// If exisits, use from session, otherwise, something like download.<mime-type-ending>	
	return &BlobBinary{Reader: reader, mimeType: bi.ContentType}
}

func (s *SharemeService) Stat(c *gae.GAEContext, key string) Share {
	bi,err := blobstore.Stat(c,appengine.BlobKey(key))
	if err != nil {
		c.HTTPContext.ReturnStatus = http.StatusNotFound
		return noShare(err)
	} else {
		return newShare(c,bi)
	}
}

//MyShares returnes a list of all shares that have been uploade by the user.
func (s *SharemeService) MyShares(c *gae.GAEContext, session *Session) []Share {
	ret := make([]Share,0,len(session.Properties))
	for k,name := range session.Properties {
		if strings.HasPrefix(k,KeySessionPrefix) {
			key := strings.TrimLeft(k,KeySessionPrefix)
			share := s.Stat(c,key)
			share.Name = name
			if !share.IsError() {
				ret = append(ret,share)
			}
		}
	}
	return ret
}

// Delete removes the object of a key from the storage. It wont be accessible afterwards.
func (s *SharemeService) Delete(c *gae.GAEContext, session *Session, key string) (ret Share) {
	ret = s.Stat(c,key)
	if ret.IsError() {
		return
	}

	session.Delete(fmt.Sprintf("%s%s",KeySessionPrefix,key))
	if err := blobstore.Delete(c,appengine.BlobKey(key)); err != nil {
		ret =  noShare(err)
	} else {
		ret = Share{Key: ret.Key,ValidPeriod: 0, Size: 0}
	}
	return
}

//Put creates a new share tihin the storage.
func (s *SharemeService) Put(c *gae.GAEContext, session *Session, name string, bc *BinaryContent) (blob Share) {
	if bc == nil {
		return noShare(errors.New("No object provided."))
	} else {
		//user := user.Current(c)

		writer, err := blobstore.Create(c, bc.MimeType())
		if err != nil {
			return noShare(err)
		}

		bi,err := io.Copy(writer,bc);
		if err != nil {
			return noShare(err)
		}

		writer.Close()
		bc.Close()

		key, err := writer.Key()
		if err != nil {
			return noShare(err)
		}

		session.Set(fmt.Sprintf("%s%s",KeySessionPrefix,key),name)

		log.Printf("BLOBSTORE created %d bytes [%s]",bi,key)
		stat := s.Stat(c,string(key))
		stat.Name = name
		return stat
	}
}

//NewSharemeService create a new service instance.
func NewSharemeService() (ret *SharemeService) {
	ret = &SharemeService{}
	return
}

//RestrictToAdmin is a GOTOJS filter that allow to restrict certain calls to the administrators
// of the appengine module/app
func RestrictToAdmin(c *gae.GAEContext) bool {
	if c == nil {
		c.HTTPContext.ReturnStatus = http.StatusInternalServerError
		log.Printf("Could not retrieve context.")
		return false
	}

	if u := user.Current(c); u != nil {
		return user.IsAdmin(c)
	} else {
		c.HTTPContext.ReturnStatus = http.StatusForbidden
		return false
	}
}

//init initializes the appengine module.
func init() {
	log.Printf("INIT()")
	f := NewFrontend()
	f.Redirect("/","/p/")
	f.ExposeInterface(NewSharemeService(),InterfaceName)
	f.EnableFileServer("htdocs","p")
	gae.SetupAndStart(f)
	//f.Bindings().Match("(Put|Delete)$").If(AutoInjectF(RestrictToAdmin))
}

