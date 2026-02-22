package hypertexting

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/sync/errgroup"

	hypertemplates "github.com/herdworks/hypertemplates-go"
)

type HyperBuild struct {
	URI         *url.URL
	ID          string
	Website     *Website
	Environment Environment
	Incremental bool

	Pages  *Pages
	Assets Counter
	Tags   Counter
	Bytes  Counter
}

func (build *HyperBuild) Execute(ctx context.Context, baseurl *url.URL, configOverrides *WebsiteConfig) (err error) {
	fmt.Printf("build \"%s\" starting\n", filepath.Base(build.URI.Path))
	start := time.Now()
	build.Reset() // reset build

	// load website
	if err = build.Website.Load(true, configOverrides); err != nil {
		return err
	}
	if err = build.Environment.Load(); err != nil {
		return err
	}
	if baseurl != nil {
		build.Website.BaseURL = baseurl.String()
	}
	if build.Website.GlobalData, err = NamespacedTemplateData(build.Website, build.Website.Config.Markdown, true); err != nil {
		return err
	}
	if build.Website.GlobalData["theme"], err = ThemeTemplateData(build.Website, build.Website.Config.Markdown, true); err != nil {
		return err
	}
	if build.ID != "" {
		build.Website.GlobalData.Set(
			"build", hypertemplates.TemplateData{
				"id":   build.ID,
				"date": time.Now().Format(time.RFC3339),
			})
	}
	if Version() != "" {
		build.Website.GlobalData.Set(
			"ht", hypertemplates.TemplateData{
				"version":      Version(),
				"release_date": ReleaseDate(),
			})
	}
	if build.Environment.TemplateData != nil {
		build.Website.GlobalData.Set("env", build.Environment.TemplateData)
	}
	if build.Website.Pages, err = GetPages(build.Website, build.Website.ContentDir()); err != nil {
		return err
	}
	if build.Website.Drafts, err = GetPages(build.Website, build.Website.DraftsDir()); err != nil {
		return err
	}

	// load pages
	if err = LoadPages(ctx, build.Website, *build.Pages, build.Website.Pages, true); err != nil {
		return err
	} // load build pages first (in case of incremental build)
	if err = LoadPages(ctx, build.Website, build.Website.Pages, *build.Pages, true); err != nil {
		return err
	} // load website pages second (sitemap)
	if build.Incremental {
		if err = AddFeedPages(ctx, build); err != nil {
			return err
		}
	} // check if build pages are in any feeds

	// load feeds
	if err = LoadPageFeeds(ctx, build.Website, *build.Pages); err != nil {
		return err
	} // populate page feeds
	if err = LoadTagPages(build); err != nil {
		return err
	} // load tag pages
	build.Pages.ResolveTemplateData(build.Website)

	// perform build
	if err = build.Validate(); err != nil {
		return err
	} // validate build
	writegroup, _ := errgroup.WithContext(ctx)
	writegroup.Go(func() error { return build.WriteAssets(ctx) })
	writegroup.Go(func() error { return build.WriteFeeds(ctx) })
	writegroup.Go(func() error { return build.WritePages(ctx) })
	if err = writegroup.Wait(); err != nil {
		return err
	}

	// finalize build
	build.BuildStats(time.Since(start))
	return nil
}

func (build *HyperBuild) Reset() {
	build.Assets.Reset() // reset assets
	build.Tags.Reset()   // reset tags
	build.Bytes.Reset()  // reset bytes
}

func (build *HyperBuild) Validate() (err error) {
	if build.URI == nil {
		return fmt.Errorf("build.uri is required")
	}
	if build.Pages == nil {
		return fmt.Errorf("build.pages is required")
	}
	if len(*build.Pages) == 0 {
		return fmt.Errorf("build.pages is empty")
	}
	return nil
}

func (build *HyperBuild) WriteAssets(ctx context.Context) (err error) {
	// copy website assets in order (website assets may override theme assets)
	if !build.Incremental {
		for _, asset := range build.Website.Assets {
			build.CopyAsset(asset)
			build.Assets.Increment(1, asset.SubPath()) // count website assets
		}
	}
	// copy page assets concurrently
	assets := hypertemplates.HyperFiles{}
	for _, page := range *build.Pages {
		assets = append(assets, page.Assets...)
	}
	build.Assets.Increment(1, assets.FilePaths()...) // count assets
	taskgroup, _ := errgroup.WithContext(ctx)
	for _, asset := range assets {
		taskgroup.Go(func() error {
			return build.CopyAsset(asset)
		})
	}
	return taskgroup.Wait()
}
func (build *HyperBuild) CopyAsset(asset hypertemplates.HyperFile) (err error) {
	var srcPath = asset.AbsPath()
	var srcFile *os.File
	var dstPath = asset.DestinationAbsPath(build.URI.Path)
	var dstFile *os.File
	var dstFileBytesWritten int64
	fmt.Printf("copying \"%s\" to \"%s\"\n", build.Website.RelativePath(srcPath), build.Website.RelativePath(dstPath))
	if err = os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}
	if srcFile, err = os.Open(srcPath); err != nil {
		return err
	} else {
		defer srcFile.Close()
	}
	if dstFile, err = os.Create(dstPath); err != nil {
		return err
	} else {
		defer dstFile.Close()
	}
	if dstFileBytesWritten, err = io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	build.Bytes.Increment(int(dstFileBytesWritten), "assets")
	return nil
}

func (build *HyperBuild) WriteFeeds(ctx context.Context) (err error) {
	taskgroup, _ := errgroup.WithContext(ctx)
	for _, page := range *build.Pages {
		taskgroup.Go(func() error {
			if page.Feed == nil {
				return nil
			} // skip non-feed pages
			return page.Feed.WriteAtom(build, page.Path)
		})
	}
	return taskgroup.Wait()
}

func (build *HyperBuild) WritePages(ctx context.Context) (err error) {
	taskgroup, _ := errgroup.WithContext(ctx)
	for i := range *build.Pages {
		taskgroup.Go(func() error {
			if (*build.Pages)[i].Draft {
				fmt.Printf("draft \"%s\" skipped\n", build.Website.RelativePath((*build.Pages)[i].URI.Path))
				return nil
			}
			return build.WritePage((*build.Pages)[i])
		})
	}
	return taskgroup.Wait()
}
func (build *HyperBuild) WritePage(page *Page) (err error) {
	var filePath string = build.BuildPath(page)
	var fileBytes []byte
	if fileBytes, err = RenderPage(build.Website, page); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}
	build.Bytes.Increment(len(fileBytes), "pages")
	return build.WriteFile(filePath, fileBytes)
}
func (build *HyperBuild) WriteFile(fileName string, fileBytes []byte) (err error) {
	var file *os.File
	var fileBytesWritten int
	fmt.Printf("writing %d bytes to %s\n", len(fileBytes), build.Website.RelativePath(fileName))
	if file, err = os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644); err != nil {
		return err
	}
	defer file.Close()
	if fileBytesWritten, err = file.Write(fileBytes); err != nil {
		return err
	}
	if fileBytesWritten != len(fileBytes) {
		return fmt.Errorf("expected to write %v bytes, but wrote %v", len(fileBytes), fileBytesWritten)
	}
	return nil
}

func (build *HyperBuild) BuildPath(page *Page) string {
	subPath := page.URI.Path                                          // the build path is embedded in page.URI
	subPath = strings.TrimPrefix(subPath, build.Website.DraftsDir())  // removes drafts dir (if draft)
	subPath = strings.TrimPrefix(subPath, build.Website.ContentDir()) // removes content dir (if regular page)
	subPath = strings.ToLower(subPath)                                // normalize case (e.g. for tag paths)
	subPath = filepath.Dir(subPath)                                   // removes index file name
	return filepath.Join(build.URI.Path, subPath, "index.html")
}

func (build *HyperBuild) BuildStats(duration time.Duration) {
	feedCount := 0
	for _, p := range *build.Pages {
		if p.Feed != nil {
			feedCount += 1
		}
	}
	fmt.Printf(
		"\nPages:  %8v (%s)\nAssets: %8v (%s)\nFeeds:  %8v (%s)\nTags:   %8v\n----------------\nTOTAL %8vms\n",
		len(*build.Pages), byteSize(build.Bytes.Counters["pages"]),
		len(build.Assets.Counters), byteSize(build.Bytes.Counters["assets"]),
		feedCount, byteSize(build.Bytes.Counters["feeds"]),
		len(build.Tags.Counters),
		duration.Milliseconds(),
	)
}
func byteSize(bytes int) string {
	if bytes < 1_024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1_048_576 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	if bytes < 1_073_741_824 {
		return fmt.Sprintf("%.1f MB", float64(bytes)/1_048_576)
	}
	return fmt.Sprintf("%.3f GB", float64(bytes)/1_073_741_824)
}

// =============================================================================
// Helper types, methods, and functions
// =============================================================================

func NamespacedTemplateData(website *Website, config *hypertemplates.HyperMarkConfig, verbose bool) (data hypertemplates.TemplateData, err error) {
	data = hypertemplates.TemplateData{}
	paths := hypertemplates.GetPathsByPattern(website.DataDir(), false, ".*\\.(yaml|json|opml)$", "")
	for _, p := range paths {
		if verbose {
			fmt.Printf("data \"%s\" loading\n", website.RelativePath(p))
		}
		var namespace hypertemplates.TemplateData
		if namespace, err = hypertemplates.ParseTemplateDataFile(p, config); err != nil {
			return data, err
		}
		data[namespace.Namespace()] = namespace.NamespaceData()
	}
	return data, nil
}

func ThemeTemplateData(website *Website, config *hypertemplates.HyperMarkConfig, verbose bool) (data hypertemplates.TemplateData, err error) {
	data = hypertemplates.TemplateData{}
	paths := hypertemplates.GetPathsByPattern(website.Theme.DataDir(), false, ".*\\.(yaml|json|opml)$", "")
	for _, p := range paths {
		if verbose {
			fmt.Printf("data \"%s\" loading\n", website.RelativePath(p))
		}
		var namespace hypertemplates.TemplateData
		if namespace, err = hypertemplates.ParseTemplateDataFile(p, config); err != nil {
			return data, err
		}
		data[namespace.Namespace()] = namespace.NamespaceData()
	}
	return data, nil
}

func NewTemplate(website *Website, page *Page) (template *hypertemplates.Template, err error) {
	var layout *html.Node
	if layout, err = website.Theme.LayoutHTML(page.LayoutRef); err != nil {
		return nil, err
	}
	return &hypertemplates.Template{
		Theme:          &website.Theme,
		Layout:         layout,
		TidyMode:       website.Config.TidyMode,
		MarkdownConfig: website.Config.Markdown,
	}, nil
}

func TemplateData(website *Website, page *Page) (data *hypertemplates.TemplateData) {
	data = &hypertemplates.TemplateData{}
	data.Set("site", *website.TemplateData)
	data.Set("data", website.GlobalData)
	data.Set("page", page.TemplateData)
	return data
}

func RenderPage(website *Website, page *Page) (pageBytes []byte, err error) {
	var template *hypertemplates.Template
	if err = page.Validate(); err != nil {
		return nil, err
	}
	if template, err = NewTemplate(website, page); err != nil {
		return nil, err
	}
	if pageBytes, err = template.Process(TemplateData(website, page)); err != nil {
		return nil, err
	}
	// page.Output = gohtml.Format(page.Output) // TODO: restore HTML formatting 🤌🏽
	return pageBytes, nil
}
