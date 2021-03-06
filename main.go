//go:generate statik -src=./static

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	pilosa "github.com/pilosa/go-pilosa"
	// ssb "github.com/pilosa/pdk/ssb"
	"github.com/spf13/pflag"
)

var Version = "v0.2.0" // demo version

var regions = map[string]int{
	"AMERICA":     0,
	"AFRICA":      1,
	"ASIA":        2,
	"EUROPE":      3,
	"MIDDLE EAST": 4,
}

var americaNations = []string{"CANADA", "ARGENTINA", "BRAZIL", "UNITED STATES", "PERU"}
var asiaNations = []string{"INDIA", "INDONESIA", "CHINA", "VIETNAM", "JAPAN"}
var asiaNationIDs = []int{10, 11, 12, 13, 14}

// 5 nations per region, in same order as above
var nations = map[string]int{
	"CANADA":         0,
	"ARGENTINA":      1,
	"BRAZIL":         2,
	"UNITED STATES":  3,
	"PERU":           4,
	"ETHIOPIA":       5,
	"ALGERIA":        6,
	"KENYA":          7,
	"MOZAMBIQUE":     8,
	"MOROCCO":        9,
	"INDIA":          10,
	"INDONESIA":      11,
	"CHINA":          12,
	"VIETNAM":        13,
	"JAPAN":          14,
	"ROMANIA":        15,
	"RUSSIA":         16,
	"FRANCE":         17,
	"UNITED KINGDOM": 18,
	"GERMANY":        19,
	"SAUDI ARABIA":   20,
	"JORDAN":         21,
	"IRAN":           22,
	"IRAQ":           23,
	"EGYPT":          24,
}

func main() {
	pilosaAddr := pflag.StringP("pilosa", "p", "localhost:10101", "host:port for pilosa")
	concurrency := pflag.IntP("concurrency", "c", 32, "number of queries to execute in parallel")
	batchSize := pflag.IntP("batchsize", "b", 1, "number of queries to combine into a single batch request")
	index := pflag.StringP("index", "i", "ssb", "pilosa index")
	pflag.Parse()

	server, err := NewServer(*pilosaAddr, *index)
	if err != nil {
		log.Fatalf("getting new server: %v", err)
	}
	server.concurrency = *concurrency
	server.batchSize = *batchSize
	fmt.Printf("Pilosa: %s\nIndex: %s\n", *pilosaAddr, *index)
	fmt.Printf("lineorder count: %d\n", server.NumLineOrders)
	server.Serve()
}

type Server struct {
	pilosaAddr    string
	Router        *mux.Router
	Client        *pilosa.Client
	Index         *pilosa.Index
	Frames        map[string]*pilosa.Frame
	concurrency   int
	batchSize     int
	NumLineOrders uint64
}

func NewServer(pilosaAddr, indexName string) (*Server, error) {
	server := &Server{
		Frames:      make(map[string]*pilosa.Frame),
		concurrency: 1,
	}

	router := mux.NewRouter()
	router.HandleFunc("/version", server.HandleVersion).Methods("GET")
	router.HandleFunc("/{qtype}/{qname}", server.HandleQuery).Methods("GET")

	pilosaURI, err := pilosa.NewURIFromAddress(pilosaAddr)
	if err != nil {
		return nil, err
	}
	client := pilosa.NewClientWithURI(pilosaURI)
	index, err := pilosa.NewIndex(indexName, nil)
	if err != nil {
		return nil, fmt.Errorf("pilosa.NewIndex: %v", err)
	}
	err = client.EnsureIndex(index)
	if err != nil {
		return nil, fmt.Errorf("client.EnsureIndex: %v", err)
	}

	// TODO should be automatic from /schema
	frames := []string{
		"lo_quantity", // these frames X each have one field, field_X
		"lo_quantity_b",
		"lo_extendedprice",
		"lo_discount",
		"lo_discount_b",
		"lo_revenue",
		"lo_supplycost",
		"lo_profit",
		"lo_revenue_computed",
		"c_city",
		"c_nation",
		"c_region",
		"s_city",
		"s_nation",
		"s_region",
		"p_mfgr",
		"p_category",
		"p_brand1",
		"lo_year",
		"lo_month",
		"lo_weeknum",
	}

	for _, frameName := range frames {
		frame, err := index.Frame(frameName, nil)
		if err != nil {
			return nil, fmt.Errorf("index.Frame %v: %v", frameName, err)
		}
		err = client.EnsureFrame(frame)
		if err != nil {
			return nil, fmt.Errorf("client.EnsureFrame %v: %v", frameName, err)
		}

		server.Frames[frameName] = frame
	}

	server.Router = router
	server.Client = client
	server.Index = index
	server.NumLineOrders = server.getLineOrderCount()
	return server, nil
}

func (s *Server) getLineOrderCount() uint64 {
	var count uint64 = 0
	for n := 0; n < 5; n++ {
		q := s.Index.Count(s.Frames["p_mfgr"].Bitmap(uint64(n)))
		response, err := s.Client.Query(q, nil)
		if err != nil {
			fmt.Printf("in getLineOrderCount: %v\n", err)
			return 666
		}
		count += response.Result().Count
	}
	return count
}

func (s *Server) HandleVersion(w http.ResponseWriter, r *http.Request) {
	if err := json.NewEncoder(w).Encode(struct {
		DemoVersion   string `json:"demoversion"`
		PilosaVersion string `json:"pilosaversion"`
	}{
		DemoVersion:   Version,
		PilosaVersion: getPilosaVersion(s.pilosaAddr),
	}); err != nil {
		log.Printf("write version response error: %s", err)
	}
}

type versionResponse struct {
	Version string `json:"version"`
}

func getPilosaVersion(host string) string {
	resp, _ := http.Get("http://" + host + "/version")
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	version := new(versionResponse)
	json.Unmarshal(body, &version)
	return version.Version
}

func (s *Server) Serve() {
	fmt.Println("Demo running at http://127.0.0.1:8000")
	log.Fatal(http.ListenAndServe(":8000", s.Router))
}
