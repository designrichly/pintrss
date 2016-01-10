package main

import (
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
)

const (
	origin  = "https://www.pinterest.com/"
	repoURL = "https://github.com/attilaolah/pinfeed"
)

var (
	thumb            = regexp.MustCompile("\\b(https?://[0-9a-z-]+.pinimg.com/)(\\d+x)(/[/0-9a-f]+.jpg)\\b")
	fullTitleBoard   = regexp.MustCompile("&gt;&lt;/a&gt;(.*?)</description>")  // Captures full title from a pinboard feed
	fullTitleChannel = regexp.MustCompile("/a&gt;&lt;/p&gt;&lt;p&gt;(.*?)&lt;/p&gt;")  // Captures full title from a channel feed
	itemsPat		 = regexp.MustCompile("</lastBuildDate>(.*?)</channel>") // Captures all <item> elements
	itemPat 		 = regexp.MustCompile("<item>(.*?)</item>")  // Captures an individual <item></item> element
	titlePat	     = regexp.MustCompile("(<title>)(.*?)(</title>)")  // Captures an individual <title></title> element
	feedPat			 = regexp.MustCompile("(.*?)<item>(?:.*)(</channel></rss>)") // Gets head and tail ignoring the whole <item></item> block
	urlReplacement   = []byte("${1}1200x${3}")
	headers          = []string{
		// Cache control headers
		"Age",
		"Cache-Control",
		"Content-Type",
		"Date",
		"Etag",
		"Last-Modified",
		"Vary",
		// Pinterest-specific stuff
		"Pinterest-Breed",
		"Pinterest-Generated-By",
		"Pinterest-Version",
	}
)

func pinFeed(w http.ResponseWriter, r *http.Request) {
	// Home page:
	if r.URL.Path == "/" {
		http.Redirect(w, r, repoURL, http.StatusMovedPermanently)
		return
	}

	// Feed pages:
	req, err := http.NewRequest(r.Method, feedURL(r.URL.Path), nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Pass along HTTP headers to Pinterest:
	for key, vals := range r.Header {
		for _, val := range vals {
			req.Header.Add(key, val)
		}
	}
	// Don't pass along the request's Accept-Encoding, enforce gzip or deflate:
	req.Header.Set("Accept-Encoding", "gzip, deflate")

	// Make an HTTP request:
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		//fmt.Println("Error in http.DefaultClient")
		//fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer res.Body.Close()

	if res.StatusCode == 304 {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	if decodeBody(res) != nil {
		//fmt.Println("Error in decodeBody")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Copy white-listed headers to the response:
	for _, key := range headers {
		if val := res.Header.Get(key); val != "" {
			w.Header().Set(key, val)
		}
	}
	w.WriteHeader(res.StatusCode)

	// store modified response
	buf, err := replaceThumbs(res.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Replace short titles with full titles
	buf2, err := replaceTitles(buf)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Write the reponse back to the caller
	w.Write(buf2)
}

func feedURL(path string) string {
	username, feed := userAndFeed(path)
	if feed == "" {
		feed = "feed"
	}
	return origin + "/" + username + "/" + feed + ".rss"
}

func userAndFeed(path string) (username, feed string) {
	path = strings.TrimSuffix(path, ".rss")
	parts := strings.SplitN(path, "/", 4)
	if len(parts) > 1 {
		username = parts[1]
	}
	if len(parts) > 2 {
		feed = parts[2]
	}
	return
}

func replaceThumbs(r io.Reader) (buf []byte, err error) {
	if buf, err = ioutil.ReadAll(r); err == nil {
		buf = thumb.ReplaceAll(buf, urlReplacement)
	}
	return
}

// TODO: Proper return of an error code
func replaceTitles(r []byte) (buf []byte, err error) {
	feedElements := feedPat.FindSubmatch(r)
	feedHead := string(feedElements[1])
	feedTail := string(feedElements[2])

	itemsElements := itemsPat.FindSubmatch(r)[1]
	s := itemPat.FindAllSubmatch(itemsElements, -1)
	for k := range s {
		// Get the full title from the description
		fullTitle, _ := getFullTitle(s[k][0])

		// Get the title elements
		title := titlePat.FindSubmatch(s[k][0])
		titlePre := string(title[1])
		titleMid := string(fullTitle)
		titlePost := string(title[3])

		// Assemble the full title element to use as the replacement
		replacementTitle := titlePre + titleMid + titlePost

		// Replace the short title with the full title
		correctedItem := titlePat.ReplaceAll(s[k][0], []byte(replacementTitle))
		
		// Append each corrected <item>
		buf = append(buf, correctedItem...)
	}
	// Re-assemble the parts of the feed
	buf = []byte(feedHead + string(buf) + feedTail)
	return buf, nil
}

// TODO: Proper return of an error code
func getFullTitle(r []byte) (buf []byte, err error) {
	if fullTitleChannel.Match(r) {
		buf := fullTitleChannel.FindSubmatch(r)[1]
		return buf, nil
	} else if fullTitleBoard.Match(r){
		buf := fullTitleBoard.FindSubmatch(r)[1]
		return buf, nil
	} else {
		return nil, nil
	}
	return buf, nil
}