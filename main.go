package main

import (
  "fmt"
  "net/http"
  "os"
  "path/filepath"
  "strings"
  "io/ioutil"
  "encoding/json"
  "net"
  "golang.org/x/net/html"
  "html/template"
)

type Config struct {
  Port string `json:"port"`
  IPRanges []string `json: "IPRanges"`
  Directory string `json:"directory"`
}

type Node struct {
  Path string
  Children []*Node
}

var config Config

func main() {
  file, _ := os.Open("config.json")
  defer file.Close()
  decoder := json.NewDecoder(file)
  err := decoder.Decode(&config)
  if err != nil {
    fmt.Println("Error: ", err)
  }

  http.HandleFunc("/", handleSearch)
  http.HandleFunc("/style.css", handleStyle)
  http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(config.Directory))))

  fmt.Println("Listening on port", config.Port)
  http.ListenAndServe(":" + config.Port, nil)
}

func handleStyle(w http.ResponseWriter, r *http.Request) {
  http.ServeFile(w, r, "style.css")
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "text/html; charset=utf-8")
  ip, _, _ := net.SplitHostPort(r.RemoteAddr)
  if !isIPInRange(ip, config.IPRanges) {
    http.Error(w, "Forbidden", http.StatusForbidden)
    fmt.Println("Forbidden access for: ", ip)
    return
  }

  query := r.URL.Query().Get("q")
  if query == "" {
    http.ServeFile(w, r, "search.html")
    return
  }

  files, err := searchFiles(config.Directory, "*.html")
  if err != nil {
    http.Error(w, "Error searching files", http.StatusInternalServerError)
    return
  }

  var results []string
  query = strings.ToLower(query) // case insensitive search
  for _, file := range files {
    content, err := ioutil.ReadFile(file)
    if err != nil {
      http.Error(w, "Error reading file", http.StatusInternalServerError)
      return
    }
    doc, err := html.Parse(strings.NewReader(string(content)))
    if err != nil {
      http.Error(w, "Error parsing HTML", http.StatusInternalServerError)
      return
    }
    text := extractText(doc)
    if strings.Contains(strings.ToLower(text), query) {
      results = append(results, "/static/"+strings.ReplaceAll(strings.TrimPrefix(file, config.Directory), "\\", "/"))
    }
  }
  
  if len(results) == 0 {
    http.Error(w, "No results found", http.StatusNotFound)
    return
  }

  root := &Node{}
  for _, result := range results {
    parts := strings.Split(result, "/")
    node := root
    for _, part := range parts {
      found := false
      for _, child := range node.Children {
        if child.Path == part {
          node = child
          found = true
          break
        }
      }
      if !found {
        newNode := &Node{Path: part}
        node.Children = append(node.Children, newNode)
        node = newNode
      }
    }
  }

  type renderFunc func(*Node, string) template.HTML
  var renderNode renderFunc
  renderNode = func(node *Node, fullPath string) template.HTML {
    if len(fullPath) > 0 {
      fullPath += "/"
    }
    fullPath += node.Path
    if len(node.Children) == 0 {
      return template.HTML(fmt.Sprintf(`<li><a href="./%s">%s</a></li>`, fullPath, node.Path))
    }
    var children string
    for _, child := range node.Children {
      children += string(renderNode(child, fullPath))
    }
    return template.HTML(fmt.Sprintf(`<li>%s<ul>%s</ul></li>`, node.Path, children))
  }

  tmpl := template.Must(template.New("results").Funcs(template.FuncMap{
    "renderNode": renderNode,
  }).Parse(`
  <!DOCTYPE html>
  <html>
  <head>
    <title>Результаты поиска</title>
    <style>
      body {
        display: flex;
        flex-direction: column;
        justify-content: center;
        align-items: center;
        #height: 100vh;
        margin: 0;
      }
      h1 {
        margin-bottom: 20px;
      }
      ul {
        text-align: left;
      }
      a:hover {
        color: #00f;
      }
    </style>
    <link rel="stylesheet" href="style.css"></link>
  </head>
  <body>
    <h1>Результаты поиска</h1>
    <ul>
    {{range .Children}}{{renderNode . ""}}{{end}}
    </ul>
  </body>
  </html>
  `))

  err = tmpl.Execute(w, struct{
    Children []*Node
    Path string
  }{
    Children: root.Children,
    Path: "",
  })
  if err != nil {
    http.Error(w, "Error generating HTML", http.StatusInternalServerError)
    return
  }
}

func extractText(n *html.Node) string {
  if n.Type == html.TextNode {
    return n.Data
  }
  var text string

  for c := n.FirstChild; c != nil; c = c.NextSibling {
    text += extractText(c)
  }
  return text
}

func isIPInRange(ip string, ranges []string) bool {
  for _, r := range ranges {
    _, ipNet, _ := net.ParseCIDR(r)
    if ipNet.Contains(net.ParseIP(ip)) {
      return true
    }
  }
  return false
}

func searchFiles(root, pattern string) ([]string, error) {
  var matches []string
  err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
    if err != nil {
      return err
    }
    if info.IsDir() {
      return nil
    }
    if matched, err := filepath.Match(pattern, filepath.Base(path)); err != nil {
      return err
    } else if matched {
      matches = append(matches, path)
    }
    return nil
  })
  if err != nil {
    return nil, err
  }
  return matches, nil
}

func readFile(path string) string {
  file, err := ioutil.ReadFile(path)
  if err != nil {
    fmt.Println(err)
  }
  return string(file)
}
