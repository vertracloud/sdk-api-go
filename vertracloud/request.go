package vertracloud

import (
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"path/filepath"
)

// addQueryParam sets key=value on q, unless value is empty — in which case
// the key is omitted entirely rather than sent as an empty string.
func addQueryParam(q url.Values, key, value string) {
	if value != "" {
		q.Set(key, value)
	}
}

// addQueryBool sets key to "true"/"false" on q, unless value is nil.
func addQueryBool(q url.Values, key string, value *bool) {
	if value == nil {
		return
	}
	if *value {
		q.Set(key, "true")
	} else {
		q.Set(key, "false")
	}
}

// multipartField is one field of a multipart/form-data request built by
// newMultipartBody. Set Reader (and FileName) for a file field; set Value
// alone for a plain text field.
type multipartField struct {
	Name        string
	Value       string
	FileName    string
	ContentType string
	Reader      io.Reader // non-nil = file field
}

// newMultipartBody streams fields into a multipart/form-data body via
// io.Pipe (no whole-file buffering — a file field is copied straight from
// its Reader into the request body as the HTTP client consumes it) and
// returns the body reader plus the Content-Type header value (with
// boundary) to pass to rest.Client.Do.
func newMultipartBody(fields []multipartField) (body io.Reader, contentType string) {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	go func() {
		writeErr := writemultipartFields(mw, fields)
		if writeErr == nil {
			writeErr = mw.Close()
		}
		pw.CloseWithError(writeErr)
	}()

	return pr, mw.FormDataContentType()
}

func writemultipartFields(mw *multipart.Writer, fields []multipartField) error {
	for _, f := range fields {
		if f.Reader == nil {
			if err := mw.WriteField(f.Name, f.Value); err != nil {
				return err
			}
			continue
		}

		ct := f.ContentType
		if ct == "" {
			ct = mime.TypeByExtension(filepath.Ext(f.FileName))
		}
		if ct == "" {
			ct = "application/octet-stream"
		}

		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
			"name":     f.Name,
			"filename": f.FileName,
		}))
		header.Set("Content-Type", ct)

		part, err := mw.CreatePart(header)
		if err != nil {
			return err
		}
		if _, err := io.Copy(part, f.Reader); err != nil {
			return err
		}
	}
	return nil
}
