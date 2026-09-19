package htxt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/herdworks/httt"
	"github.com/herdworks/hyperfeed/reader"
)

func sampleContact() *Contact {
	return &Contact{
		Name:   " Caleb Hailey ",
		Email:  []ContactEmail{{Label: "default", Address: "me@example.com", Preferred: true}, {Label: "comments", Address: "hello@example.com", Types: []string{"work"}}},
		Phone:  []ContactPhone{{Label: "iPhone", Address: "+15032094070", Types: []string{"cell", "voice"}}},
		Social: []ContactSocialProfile{{Service: "Instagram", Username: "calebhailey", Href: "https://instagram.com/calebhailey", UserIdentifier: "123"}, {Service: "Custom Service", Username: "user/name:一", Label: "profile"}},
		Links:  []ContactLink{{Label: "iMessage", Href: "sms://me@example.com"}, {Label: "FaceTime", Href: "facetime://me@example.com"}},
	}
}

func TestContactConfigRoundTripAndTemplateData(t *testing.T) {
	for _, extension := range []string{"yaml", "json"} {
		t.Run(extension, func(t *testing.T) {
			fixture := newBuildExecutionFixture(t)
			original, err := os.ReadFile(fixture.sitePath)
			if err != nil {
				t.Fatal(err)
			}
			var website Website
			if err := yaml.Unmarshal(original, &website); err != nil {
				t.Fatal(err)
			}
			website.Contact = sampleContact()
			var data []byte
			if extension == "yaml" {
				data, err = yaml.Marshal(&website)
			} else {
				data, err = json.Marshal(&website)
			}
			if err != nil {
				t.Fatal(err)
			}
			fixture.sitePath = filepath.Join(fixture.root, "site."+extension)
			if err := os.WriteFile(fixture.sitePath, data, 0600); err != nil {
				t.Fatal(err)
			}
			build := fixture.build()
			if err := build.LoadContext(t.Context(), io.Discard); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(build.Website.Contact, website.Contact) {
				t.Fatalf("round trip: %#v", build.Website.Contact)
			}
			var contact httt.TemplateData
			switch value := build.Website.TemplateData.Get("contact").(type) {
			case httt.TemplateData:
				contact = value
			case map[string]any:
				contact = httt.TemplateData(value)
			}
			if contact.Get("name") != website.Contact.Name {
				t.Fatalf("template data: %#v", build.Website.TemplateData.Get("contact"))
			}
			for _, key := range []string{"email", "phone", "social", "links"} {
				if contact.Get(key) == nil {
					t.Fatalf("missing template collection %s", key)
				}
			}
		})
	}
	schema := string(schemaDocument(t, "website"))
	for _, field := range []string{"contact", "user_identifier", "preferred", "social", "links"} {
		if !strings.Contains(schema, `"`+field+`"`) {
			t.Errorf("schema missing %s", field)
		}
	}
}

func TestContactVCard(t *testing.T) {
	contact := sampleContact()
	data, err := contact.vcard(nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(data), "\r\n ", "")
	for _, want := range []string{
		"BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Caleb Hailey\r\nN:;;;;\r\n",
		"item1.EMAIL;TYPE=PREF:me@example.com\r\nitem1.X-ABLabel:default",
		"item2.EMAIL;TYPE=WORK:hello@example.com",
		"item3.TEL;TYPE=CELL,VOICE:+15032094070\r\nitem3.X-ABLabel:iPhone",
		"X-SOCIALPROFILE;TYPE=Instagram;X-USER=calebhailey;X-USERID=123:https://instagram.com/calebhailey",
		"item4.X-SOCIALPROFILE;TYPE=\"Custom Service\";X-USER=\"user/name:一\":x-apple:user%2Fname:%E4%B8%80",
		"item5.URL:sms://me@example.com", "item6.URL:facetime://me@example.com", "END:VCARD\r\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
	again, err := contact.vcard(nil, "", "")
	if err != nil || !bytes.Equal(data, again) {
		t.Fatal("nondeterministic encoding")
	}
	contact.Name = strings.Repeat("é一", 40) + `,semicolon;backslash\`
	data, err = contact.vcard(nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\r\n") {
		if len(line) > 75 || !utf8.ValidString(line) {
			t.Fatalf("bad folded line %q", line)
		}
	}
	unfolded := strings.ReplaceAll(string(data), "\r\n ", "")
	if !strings.Contains(unfolded, `\,semicolon\;backslash\\`) {
		t.Fatalf("unescaped name: %s", unfolded)
	}
	if strings.Contains(strings.ReplaceAll(string(data), "\r\n", ""), "\n") {
		t.Fatal("bare LF")
	}
}

func TestContactValidation(t *testing.T) {
	for _, contact := range []*Contact{nil, {}, {Name: " \t\n", Email: []ContactEmail{{Address: "bad"}}}} {
		data, err := contact.vcard(nil, "", "")
		if err != nil || len(data) != 0 {
			t.Fatalf("disabled contact: %q %v", data, err)
		}
	}
	for name, modify := range map[string]func(*Contact){
		"name injection":         func(c *Contact) { c.Name = "Name\r\nEMAIL:bad@example.com" },
		"blank email":            func(c *Contact) { c.Email[0].Address = " " },
		"bad email":              func(c *Contact) { c.Email[0].Address = "invalid" },
		"email display name":     func(c *Contact) { c.Email[0].Address = "Name <me@example.com>" },
		"blank phone":            func(c *Contact) { c.Phone[0].Address = "" },
		"bad phone":              func(c *Contact) { c.Phone[0].Address = "not a number" },
		"label injection":        func(c *Contact) { c.Phone[0].Label = "a\nTEL:123" },
		"type injection":         func(c *Contact) { c.Phone[0].Types = []string{"cell:123"} },
		"implicit preference":    func(c *Contact) { c.Phone[0].Types = []string{"pref"} },
		"blank service":          func(c *Contact) { c.Social[0].Service = " " },
		"missing social address": func(c *Contact) { c.Social[0].Href = ""; c.Social[0].Username = "" },
		"parameter injection":    func(c *Contact) { c.Social[0].Username = `a";TYPE=other` },
		"relative URI":           func(c *Contact) { c.Links[0].Href = "relative/path" },
		"bad URI":                func(c *Contact) { c.Links[0].Href = "https://" },
		"URI whitespace":         func(c *Contact) { c.Links[0].Href = "sms:a b" },
		"URI injection":          func(c *Contact) { c.Links[0].Href = "sms:a\r\nFN:other" },
	} {
		t.Run(name, func(t *testing.T) {
			c := sampleContact()
			modify(c)
			if c.Validate() == nil {
				t.Fatal("accepted invalid contact")
			}
		})
	}
}

func TestContactPhoto(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	if err := build.LoadContext(t.Context(), io.Discard); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	for _, format := range []string{"PNG", "JPEG"} {
		var photo bytes.Buffer
		var err error
		if format == "PNG" {
			err = png.Encode(&photo, img)
		} else {
			err = jpeg.Encode(&photo, img, nil)
		}
		if err != nil {
			t.Fatal(err)
		}
		// Deliberately use an unrelated extension: content determines the media type.
		path := filepath.Join(fixture.root, "photo.bin")
		if err := os.WriteFile(path, photo.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		build.Website.Contact = &Contact{Name: "Photo", Favicon: "photo.bin"}
		data, err := build.Website.contactVCard(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		text := strings.ReplaceAll(string(data), "\r\n ", "")
		_, value, ok := strings.Cut(text, "PHOTO;ENCODING=b;TYPE="+format+":")
		if !ok {
			t.Fatalf("missing photo type %s", format)
		}
		value, _, _ = strings.Cut(value, "\r\n")
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil || !bytes.Equal(decoded, photo.Bytes()) {
			t.Fatal("photo bytes changed")
		}
	}
	if err := os.WriteFile(filepath.Join(fixture.root, "bad.svg"), []byte("<svg/>"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"missing.png", "bad.svg", "../outside.png", "/tmp/photo.png", "ftp://example.com/photo.png"} {
		build.Website.Contact.Favicon = path
		if _, err := build.Website.contactVCard(t.Context()); err == nil || !strings.Contains(err.Error(), "site.contact.favicon") {
			t.Fatalf("path %q: %v", path, err)
		}
	}
}

func TestContactBuildLifecycle(t *testing.T) {
	for _, incremental := range []bool{false, true} {
		for _, ordinary := range []bool{false, true} {
			t.Run(strings.Join([]string{map[bool]string{false: "full", true: "incremental"}[incremental], map[bool]string{false: "no-static-contact", true: "static-contact"}[ordinary]}, "/"), func(t *testing.T) {
				fixture := newBuildExecutionFixture(t)
				original, err := os.ReadFile(fixture.sitePath)
				if err != nil {
					t.Fatal(err)
				}
				if ordinary {
					writeBuildSelectionFile(t, filepath.Join(fixture.root, "static", contactAssetPath), "ordinary contact")
				}
				if err := os.WriteFile(fixture.sitePath, append(original, []byte("\ncontact:\n  name: Name Only\n")...), 0600); err != nil {
					t.Fatal(err)
				}
				build := fixture.build()
				if incremental {
					build.Config.Incremental = true
					build.Config.Page = "content/index.html"
				}
				if err := build.Execute(t.Context()); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(fixture.buildPath, contactAssetPath)
				data, err := os.ReadFile(path)
				if err != nil || !bytes.Contains(data, []byte("FN:Name Only\r\nN:;;;;")) {
					t.Fatalf("generated contact: %s %v", data, err)
				}
				if got := build.Status.Bytes.Map()["generated_assets"]; got != len(data)+len("test\n") {
					t.Fatalf("bytes = %d", got)
				}
				if build.Status.Assets.Map()[contactAssetPath] < 1 {
					t.Fatal("contact not counted")
				}
				// Reuse the same output path, with a blank name and otherwise invalid data.
				if err := os.WriteFile(fixture.sitePath, append(original, []byte("\ncontact:\n  name: ' '\n  favicon: missing.png\n")...), 0600); err != nil {
					t.Fatal(err)
				}
				if err := build.Execute(t.Context()); err != nil {
					t.Fatal(err)
				}
				want := data
				if ordinary && !incremental {
					// Ordinary full-build asset copying still overwrites this path.
					want = []byte("ordinary contact")
				}
				data, err = os.ReadFile(path)
				if err != nil || !bytes.Equal(data, want) {
					t.Fatalf("disabled generation changed existing output: %q %v", data, err)
				}
				if got := build.Status.Bytes.Map()["generated_assets"]; got != len("test\n") {
					t.Fatalf("disabled bytes = %d", got)
				}
				marker, err := os.ReadFile(filepath.Join(fixture.buildPath, ".well-known/hypertexting/build"))
				if err != nil || string(marker) != "test\n" {
					t.Fatalf("marker: %s %v", marker, err)
				}
			})
		}
	}
}

func TestContactBuildReportsPhotoError(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	original, err := os.ReadFile(fixture.sitePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.sitePath, append(original, []byte("\ncontact:\n  name: Missing Image\n  favicon: missing.png\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if err := fixture.build().Execute(t.Context()); err == nil || !strings.Contains(err.Error(), "site.contact.favicon") {
		t.Fatalf("expected field-specific build error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.buildPath, contactAssetPath)); !os.IsNotExist(err) {
		t.Fatalf("failed build wrote a contact file: %v", err)
	}
}

func TestContactRemotePhoto(t *testing.T) {
	var photo bytes.Buffer
	if err := png.Encode(&photo, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/photo", "/favicon%name":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(photo.Bytes())
		case "/redirect":
			http.Redirect(w, r, "/photo", http.StatusFound)
		case "/local":
			http.Redirect(w, r, "file:///tmp/photo.png", http.StatusFound)
		case "/invalid":
			_, _ = w.Write([]byte("not an image"))
		case "/large":
			_, _ = w.Write(bytes.Repeat([]byte("x"), (3<<20)+1))
		case "/failure":
			http.Error(w, "failed", http.StatusInternalServerError)
		case "/slow":
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	})
	for _, tls := range []bool{false, true} {
		name := "http"
		if tls {
			name = "https"
		}
		t.Run(name, func(t *testing.T) {
			var server *httptest.Server
			if tls {
				server = httptest.NewTLSServer(handler)
				// Trust only the test server through the default transport cloned by reader.
				original := http.DefaultTransport
				transport := original.(*http.Transport).Clone()
				transport.TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
				http.DefaultTransport = transport
				t.Cleanup(func() { http.DefaultTransport = original; transport.CloseIdleConnections() })
			} else {
				server = httptest.NewServer(handler)
			}
			defer server.Close()
			fixture := newBuildExecutionFixture(t)
			build := fixture.build()
			if err := build.LoadContext(t.Context(), io.Discard); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"/photo", "/redirect", "/favicon%25name"} {
				build.Website.Contact = &Contact{Name: "Remote", Favicon: server.URL + path}
				if err := build.WriteGeneratedAssets(io.Discard); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(filepath.Join(fixture.buildPath, contactAssetPath))
				if err != nil {
					t.Fatal(err)
				}
				unfolded := strings.ReplaceAll(string(data), "\r\n ", "")
				if !strings.Contains(unfolded, "PHOTO;ENCODING=b;TYPE=PNG:"+base64.StdEncoding.EncodeToString(photo.Bytes())+"\r\n") {
					t.Fatal("remote photo not embedded verbatim")
				}
			}
			for _, path := range []string{"/missing", "/failure", "/invalid", "/large", "/local"} {
				build.Website.Contact.Favicon = server.URL + path
				_, err := build.Website.contactVCard(t.Context())
				if err == nil || !strings.Contains(err.Error(), "site.contact.favicon") {
					t.Fatalf("%s: %v", path, err)
				}
				if path == "/large" && !errors.Is(err, reader.ErrContentLength) {
					t.Fatalf("expected size limit: %v", err)
				}
			}
			build.Website.Contact.Favicon = server.URL + "/slow"
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			if err := build.writeGeneratedAssets(ctx, io.Discard); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("request deadline not propagated: %v", err)
			}
			build.Website.Contact.Name = " "
			if data, err := build.Website.contactVCard(t.Context()); err != nil || len(data) != 0 {
				t.Fatalf("disabled contact fetched image: %s %v", data, err)
			}
		})
	}
}

func TestContactLocalPhotoSpecialFilename(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	if err := build.LoadContext(t.Context(), io.Discard); err != nil {
		t.Fatal(err)
	}
	var photo bytes.Buffer
	if err := png.Encode(&photo, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	name := "photo #1%?.png"
	if err := os.WriteFile(filepath.Join(fixture.root, name), photo.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	build.Website.Contact = &Contact{Name: "Local", Favicon: name}
	if _, err := build.Website.contactVCard(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestContactNicknameAndTypeMetadata(t *testing.T) {
	for _, test := range []struct{ baseURL, nickname string }{
		{"https://calebhailey.com/", "@calebhailey.com"},
		{"https://www.Example.com:8443/blog/?q=1#fragment", "@example.com"},
		{"http://localhost:8080/", "@localhost"},
		{"", ""},
	} {
		website := &Website{BaseURL: test.baseURL, Contact: &Contact{
			Name:   "Caleb Hailey",
			Email:  []ContactEmail{{Label: "Personal", Address: "me@calebhailey.com", Types: []string{"internet", "home"}, Preferred: true}},
			Phone:  []ContactPhone{{Label: "iPhone", Address: "+15032094070", Types: []string{"cell", "voice"}, Preferred: true}},
			Social: []ContactSocialProfile{{Service: "Signal", Username: "calebhailey.01"}, {Service: "iMessage", Username: "me@calebhailey.com"}},
			Links:  []ContactLink{{Label: "Website", Href: "https://calebhailey.com/", Types: []string{"home"}, Preferred: true}},
		}}
		data, err := website.contactVCard(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		text := strings.ReplaceAll(string(data), "\r\n ", "")
		if test.nickname != "" {
			if !strings.Contains(text, "NICKNAME:"+test.nickname+"\r\n") {
				t.Fatalf("nickname for %s: %s", test.baseURL, text)
			}
		} else if strings.Contains(text, "NICKNAME:") {
			t.Fatal("unexpected nickname")
		}
		for _, want := range []string{
			"item1.EMAIL;TYPE=INTERNET,HOME,PREF:me@calebhailey.com\r\nitem1.X-ABLabel:Personal",
			"item2.TEL;TYPE=CELL,VOICE,PREF:+15032094070\r\nitem2.X-ABLabel:iPhone",
			"X-SOCIALPROFILE;TYPE=Signal;X-USER=calebhailey.01:x-apple:calebhailey.01",
			"X-SOCIALPROFILE;TYPE=iMessage;X-USER=me@calebhailey.com:x-apple:me@calebhailey.com",
			"item3.URL;TYPE=HOME,PREF:https://calebhailey.com/\r\nitem3.X-ABLabel:Website",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("missing %q in %s", want, text)
			}
		}
	}
}

func TestContactValidationAtWebsiteLoad(t *testing.T) {
	var requests atomic.Int32
	var photo bytes.Buffer
	if err := png.Encode(&photo, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write(photo.Bytes())
	}))
	defer server.Close()
	fixture := newBuildExecutionFixture(t)
	original, err := os.ReadFile(fixture.sitePath)
	if err != nil {
		t.Fatal(err)
	}
	config := "\ncontact:\n  name: Test\n  favicon: " + server.URL + "/photo.png\n  email:\n    - address: invalid\n"
	if err := os.WriteFile(fixture.sitePath, append(original, []byte(config)...), 0600); err != nil {
		t.Fatal(err)
	}
	build := fixture.build()
	if err := build.LoadContext(t.Context(), io.Discard); err == nil || !strings.Contains(err.Error(), "site.contact.email[0]") {
		t.Fatalf("loading did not reject invalid contact: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatal("invalid contact fetched image during loading")
	}
	config = strings.Replace(config, "address: invalid", "address: hello@example.com", 1)
	if err := os.WriteFile(fixture.sitePath, append(original, []byte(config)...), 0600); err != nil {
		t.Fatal(err)
	}
	build = fixture.build()
	if err := build.LoadContext(t.Context(), io.Discard); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 {
		t.Fatal("valid contact fetched image during loading")
	}
	if err := build.WriteGeneratedAssets(io.Discard); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("image requests = %d, want 1", requests.Load())
	}
}

func TestContactFaviconURLValidationAtLoad(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	original, err := os.ReadFile(fixture.sitePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"https://", "http://bad%host/photo.png", "https://example.com/bad path.png"} {
		t.Run(source, func(t *testing.T) {
			var website Website
			if err := yaml.Unmarshal(original, &website); err != nil {
				t.Fatal(err)
			}
			website.Contact = &Contact{Name: "Test", Favicon: source}
			data, err := yaml.Marshal(website)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(fixture.sitePath, data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := fixture.build().LoadContext(t.Context(), io.Discard); err == nil || !strings.Contains(err.Error(), "site.contact.favicon") {
				t.Fatalf("loading did not reject invalid favicon URL: %v", err)
			}
		})
	}
	for _, source := range []string{"https://example.com/photo.png", "HTTP://example.com/photo.png", "static/photo #1%.png"} {
		contact := &Contact{Name: "Test", Favicon: source}
		if err := contact.Validate(); err != nil {
			t.Fatalf("valid source %q: %v", source, err)
		}
	}
}

func TestContactPhotoDimensionLimit(t *testing.T) {
	fixture := newBuildExecutionFixture(t)
	build := fixture.build()
	if err := build.LoadContext(t.Context(), io.Discard); err != nil {
		t.Fatal(err)
	}
	build.Website.Contact = &Contact{Name: "Photo", Favicon: "photo.bin"}
	for _, format := range []string{"PNG", "JPEG"} {
		for _, test := range []struct {
			name               string
			width, height      int
			oversized, corrupt bool
		}{
			{"at-limit", 4096, 4096, false, false},
			{"too-wide", 4097, 1, true, false},
			{"too-tall", 1, 4097, true, false},
			{"oversized-corrupt-body", 4097, 1, true, true},
			{"small-corrupt-body", 16, 16, false, true},
		} {
			t.Run(format+"/"+test.name, func(t *testing.T) {
				var encoded bytes.Buffer
				img := image.NewGray(image.Rect(0, 0, test.width, test.height))
				var err error
				if format == "PNG" {
					err = png.Encode(&encoded, img)
				} else {
					err = jpeg.Encode(&encoded, img, nil)
				}
				if err != nil {
					t.Fatal(err)
				}
				photo := encoded.Bytes()
				if len(photo) > 3<<20 {
					t.Fatal("fixture exceeds input byte limit")
				}
				if test.corrupt {
					photo = photo[:len(photo)-10]
				}
				if _, _, err := image.DecodeConfig(bytes.NewReader(photo)); err != nil {
					t.Fatalf("fixture must have valid header: %v", err)
				}
				if err := os.WriteFile(filepath.Join(fixture.root, "photo.bin"), photo, 0600); err != nil {
					t.Fatal(err)
				}
				data, err := build.Website.contactVCard(t.Context())
				if test.oversized {
					if err == nil || !strings.Contains(err.Error(), "site.contact.favicon") || !strings.Contains(err.Error(), "4096x4096 pixel limit") {
						t.Fatalf("expected dimension error before full decoding: %v", err)
					}
				} else if test.corrupt {
					if err == nil || !strings.Contains(err.Error(), "must be a valid JPEG or PNG") {
						t.Fatalf("expected corrupt body error: %v", err)
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					unfolded := strings.ReplaceAll(string(data), "\r\n ", "")
					if !strings.Contains(unfolded, "PHOTO;ENCODING=b;TYPE="+format+":"+base64.StdEncoding.EncodeToString(photo)+"\r\n") {
						t.Fatal("accepted photo bytes changed")
					}
				}
			})
		}
	}
}
