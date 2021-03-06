package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"

	"github.com/akhenakh/regionagogo"
	"github.com/akhenakh/regionagogo/db/boltdb"
	pb "github.com/akhenakh/regionagogo/regionagogosvc"
	"google.golang.org/grpc"
)

type server struct {
	regionagogo.GeoFenceDB
}

func (s *server) GetRegion(ctx context.Context, p *pb.Point) (*pb.RegionResponse, error) {
	region, err := s.StubbingQuery(float64(p.Latitude), float64(p.Longitude))
	if err != nil {
		return nil, err
	}
	if region == nil || len(region) == 0 {
		return &pb.RegionResponse{Code: "unknown"}, nil
	}

	// default is to lookup for "iso"
	iso, ok := region[0].Data["iso"]
	if !ok {
		return &pb.RegionResponse{Code: "unknown"}, nil
	}

	rs := pb.RegionResponse{Code: iso}
	return &rs, nil
}

// queryHandler takes a lat & lng query params and return a JSON
// with the country of the coordinate
func (s *server) queryHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	slat := query.Get("lat")
	lat, err := strconv.ParseFloat(slat, 64)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	slng := query.Get("lng")
	lng, err := strconv.ParseFloat(slng, 64)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	fences, err := s.StubbingQuery(lat, lng)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	if len(fences) < 1 {
		js, _ := json.Marshal(map[string]string{"name": "unknown"})
		w.Write(js)
		return
	}

	js, _ := json.Marshal(fences[0].Data)
	w.Write(js)
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	dbpath := flag.String("dbpath", "", "Database path")
	debug := flag.Bool("debug", false, "Enable debug")
	httpPort := flag.Int("httpPort", 8082, "http debug port to listen on")
	grpcPort := flag.Int("grpcPort", 8083, "grpc port to listen on")
	cachedEntries := flag.Uint("cachedEntries", 0, "Region Cache size, 0 for disabled")

	flag.Parse()
	opts := []boltdb.GeoFenceBoltDBOption{
		boltdb.WithCachedEntries(*cachedEntries),
		boltdb.WithDebug(*debug),
	}
	gs, err := boltdb.NewGeoFenceBoltDB(*dbpath, opts...)
	if err != nil {
		log.Fatal(err)
	}

	s := &server{GeoFenceDB: gs}
	http.HandleFunc("/query", s.queryHandler)
	go func() {
		log.Println(http.ListenAndServe(fmt.Sprintf(":%d", *httpPort), nil))
	}()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *grpcPort))

	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterRegionAGogoServer(grpcServer, s)
	grpcServer.Serve(lis)
}
