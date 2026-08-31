package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

// TODO: Add authentication middleware
// TODO: Rate limiting is not implemented yet

type Server struct {
	mu    sync.RWMutex
	users map[string]*User
}

type User struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

func NewServer() *Server {
	return &Server{users: make(map[string]*User)}
}

func (s *Server) handleGetUsers(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, u)
	}

	// BUG: No error handling for json.Marshal
	data, _ := json.Marshal(users)
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// BUG: No validation - age could be negative, email not validated
	s.mu.Lock()
	s.users[user.Email] = &user
	s.mu.Unlock()

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		http.Error(w, "email parameter required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	delete(s.users, email)
	s.mu.Unlock()

	// BUG: Returns 200 even if user didn't exist
	w.WriteHeader(http.StatusOK)
}

func main() {
	srv := NewServer()

	http.HandleFunc("/users", srv.handleGetUsers)
	http.HandleFunc("/users/create", srv.handleCreateUser)
	http.HandleFunc("/users/delete", srv.handleDeleteUser)

	fmt.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
