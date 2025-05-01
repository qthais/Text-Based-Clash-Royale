package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strings"
	"time"
)

type User struct {
	Username  string
	Password  string
	Fullname  string
	Emails    []string
	Addresses []string
}

type Word struct {
	Word        string `json:"word"`
	Description string `json:"description"`
}

type Player struct {
	Username string
	Conn     net.Conn
}

func HashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

func saveUsers(users []User, filename string) error {
	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

func loadUsers(filename string) ([]User, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var users []User
	err = json.Unmarshal(data, &users)
	return users, err
}

func authenticateUser(username, password string, users []User) (bool, User) {
	hashed := HashPassword(password)
	for _, user := range users {
		if user.Username == username && user.Password == hashed {
			return true, user
		}
	}
	return false, User{}
}

func contains(s []rune, e rune) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func handleClient(conn net.Conn, users []User, players chan Player) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	var ok bool
	var user User
	// Ask for username and password
	for {

		writer.Write([]byte("Username:"))
		writer.Flush()
		username, _ := reader.ReadString('\n')
		username = strings.TrimSpace(username)
		writer.Write([]byte("Password: "))
		writer.Flush()
		password, _ := reader.ReadString('\n')
		password = strings.TrimSpace(password)

		ok, user = authenticateUser(username, password, users)
		if !ok {
			writer.Write([]byte("Authentication failed.\n"))
			writer.Flush()
			continue
		} else {
			newPlayer := Player{Username: user.Username, Conn: conn}
			players <- newPlayer
			break
		}
	}

	writer.WriteString(fmt.Sprintf("Welcome, %s! You're now connected.\n", user.Fullname))
	writer.Flush()

}
func chat(player1, player2 Player) {
	fmt.Println("Starting chat between", player1.Username, "and", player2.Username)

	// Inform both players that the game/chat is starting
	player1.Conn.Write([]byte("Both players are connected! You can now start chatting.\n"))
	player2.Conn.Write([]byte("Both players are connected! You can now start chatting.\n"))

	// Start the chat loop
	go messageLoop(player1, player2)
	messageLoop(player2, player1) // Run one loop in the current goroutine
}

func messageLoop(sender Player, receiver Player) {
	reader := bufio.NewReader(sender.Conn)
	writer := bufio.NewWriter(sender.Conn)

	for {
		// Read the message from the sender
		message, err := reader.ReadString('\n')
		if err != nil {
			// If the error is a connection closed error, notify and break the loop
			if err.Error() == "use of closed network connection" {
				fmt.Println(sender.Username, "disconnected.")
				sender.Conn.Write([]byte("You have been disconnected.\n"))
				sender.Conn.Close()
				return
			}
			fmt.Println("Error reading message:", err)
			return
		}

		message = strings.TrimSpace(message)

		if message == "exit" {
			sender.Conn.Write([]byte("You have left the chat.\n"))
			sender.Conn.Close()
			return
		}

		receiver.Conn.Write([]byte(fmt.Sprintf("%s: %s\n", sender.Username, message)))
		writer.Flush()
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())

	// Load users
	users, err := loadUsers("users.json")
	if err != nil {
		fmt.Println("Error loading users:", err)
		return
	}

	// Start server
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Println("Error starting server:", err)
		return
	}
	defer listener.Close()

	fmt.Println("Server listening on port 8080...")
	players := make(chan Player)
	for {
		conn, err := listener.Accept()
		fmt.Print(conn)
		if err != nil {
			fmt.Println("Connection error:", err)
			continue
		}

		go handleClient(conn, users, players)

		// Wait for two players by receiving from the channel twice
		go func() {
			player1 := <-players
			player2 := <-players

			// Start chat with these two players
			go chat(player1, player2)
		}()
	}
}
