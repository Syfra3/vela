package sample

import "fmt"

type Server struct {
	host string
	port int
}

type Handler interface {
	Handle(req string) error
}

func NewServer(host string, port int) *Server {
	return &Server{host: host, port: port}
}

func (s *Server) Start() error {
	fmt.Printf("starting on %s:%d\n", s.host, s.port)
	s.listen()
	return nil
}

func (s *Server) listen() {
	fmt.Println("listening")
}
