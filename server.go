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

type TowerStats struct {
	HP   int
	ATK  int
	DEF  int
	CRIT float32
	EXP  int
}
type TroopStats struct {
	HP   int
	ATK  int
	DEF  int
	MANA int
	EXP  int
}

type TowerType string
type TroopType string

const (
	KingTower  TowerType = "King"
	GuardTower TowerType = "Guard"
)
const (
	PawnTroop   TroopType = "Pawn"
	BishopTroop TroopType = "Bishop"
	RookTroop   TroopType = "Rook"
	KnightTroop TroopType = "Knight"
	PrinceTroop TroopType = "Prince"
	QueenTroop  TroopType = "Queen"
)

type Troop struct {
	ID    string
	Name  string
	Type  TroopType
	Stats TroopStats
}

type Tower struct {
	ID       string
	Type     TowerType
	Position int //0 for King; 1,2 for Guard
	Stats    TowerStats
	IsAlive  bool
}
type GameState struct {
	Player1 *Player
	Player2 *Player
	Turn    int // Alternates between 1 and 2
}

type User struct {
	Username  string
	Password  string
	Fullname  string
	Emails    []string
	Addresses []string
}

func CalculateDamage(attacker TroopStats, defender TowerStats) int {
	isCritical := rand.Float32() < defender.CRIT
	attack := attacker.ATK
	if isCritical {
		additionalDamage := float32(attack) * 1.2
		attack = attack + int(additionalDamage)
	}
	damage := attack - defender.DEF
	if damage < 0 {
		return 0
	}
	return damage
}

type Player struct {
	Username string
	Conn     net.Conn
	Towers   []Tower
	Troops   []Troop
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
	fmt.Println("Starting game between", player1.Username, "and", player2.Username)

	// Inform both players that the game/chat is starting
	player1.Conn.Write([]byte("Both players are connected! You can now start playing.\n"))
	player2.Conn.Write([]byte("Both players are connected! You can now start playing.\n"))

	// Start the chat loop
	go gameLoop(player1, player2)
}

func gameLoop(player1, player2 Player) {
	game := GameState{
		Player1: &player1,
		Player2: &player2,
		Turn:    1,
	}

	// Initialize towers and troops for both players
	// (you'll need to implement this)
	setupPlayerAssets(game.Player1)
	setupPlayerAssets(game.Player2)
	sendTroopPool(&player1)
	sendTroopPool(&player2)

	player1.Conn.Write([]byte("Game started! You are Player 1.\n"))
	player2.Conn.Write([]byte("Game started! You are Player 2.\n"))

	for {
		var currentPlayer, opponent *Player
		var destroyFlag bool = false
		if game.Turn == 1 {
			currentPlayer = game.Player1
			opponent = game.Player2
		} else {
			currentPlayer = game.Player2
			opponent = game.Player1
		}
		for {

			currentPlayer.Conn.Write([]byte("Your turn. Type the name of your troop and target tower (e.g., Goblin G1):\n"))
			opponent.Conn.Write([]byte("Waiting for the other player's move...\n"))

			reader := bufio.NewReader(currentPlayer.Conn)
			input, err := reader.ReadString('\n')
			if err != nil {
				// fmt.Println("Connection lost with", currentPlayer.Username)
				break
			}

			input = strings.TrimSpace(input)
			if input == "exit" {
				currentPlayer.Conn.Write([]byte("You exited the game.\n"))
				break
			}

			destroyed, moveResult := applyMove(currentPlayer, opponent, input)
			destroyFlag = destroyed
			if strings.HasPrefix(moveResult, "Invalid") {
				continue
			}
			break
		}

		if isGameOver(opponent) {
			currentPlayer.Conn.Write([]byte("You win!\n"))
			opponent.Conn.Write([]byte("You lose.\n"))
			break
		}

		if !destroyFlag {
			if game.Turn == 1 {
				game.Turn = 2
			} else {
				game.Turn = 1
			}
		}
	}
}
func sendTroopPool(player *Player) {
	troopPoolMessage := "Your available Troops:\n"

	// Loop through the player's randomly selected troops
	for i, troop := range player.Troops {
		troopPoolMessage += fmt.Sprintf("%d. %s: HP = %d, ATK = %d, DEF = %d, MANA = %d, EXP = %d\n",
			i+1, troop.Name, troop.Stats.HP, troop.Stats.ATK, troop.Stats.DEF, troop.Stats.MANA, troop.Stats.EXP)
	}

	// Send the troop details to the player
	player.Conn.Write([]byte(troopPoolMessage))
}

func applyMove(currentPlayer *Player, opponent *Player, input string) (bool, string) {
	parts := strings.Fields(input)

	if len(parts) != 2 {
		msg := "Invalid move format. Use 'TroopName TowerType'.\n"
		currentPlayer.Conn.Write([]byte(msg))
		return false, msg
	}
	troopName := parts[0]
	towerType := parts[1]
	var troop *Troop
	for i := range currentPlayer.Troops {
		if currentPlayer.Troops[i].Name == troopName {
			troop = &currentPlayer.Troops[i]
			break
		}
	}
	if troop == nil {
		msg := fmt.Sprintf("Invalid attack, Troop %s not found.\n", troopName)
		currentPlayer.Conn.Write([]byte(msg))
		return false, msg
	}
	var targetTower *Tower
	for i := range opponent.Towers {
		if opponent.Towers[i].ID == towerType {
			targetTower = &opponent.Towers[i]
			break
		}
	}
	if targetTower == nil {
		msg := fmt.Sprintf("Invalid attack, Target tower %s not found.\n", towerType)
		currentPlayer.Conn.Write([]byte(msg))
		return false, msg
	}
	if !targetTower.IsAlive {
		msg := fmt.Sprintf("Invalid attack, Tower %s is already destroyed.\n", towerType)
		currentPlayer.Conn.Write([]byte(msg))
		return false, msg
	}

	isValid, msg := isValidAttack(targetTower, opponent)
	if !isValid {
		currentPlayer.Conn.Write([]byte(msg))
		return false, msg
	} else {
		damage := CalculateDamage(troop.Stats, targetTower.Stats)
		targetTower.Stats.HP -= damage
		destroyed := false
		if targetTower.Stats.HP <= 0 {
			targetTower.IsAlive = false
			targetTower.Stats.HP = 0
			destroyed = true
		}
		result := fmt.Sprintf("You attacks %s with %s!\n", towerType, troopName)
		if targetTower.IsAlive {
			result += fmt.Sprintf("Opponent's %s tower has %d HP left.\n", towerType, targetTower.Stats.HP)
		} else {
			result += fmt.Sprintf("Opponent's %s tower is destroyed!\n", towerType)
		}

		opponentResult := fmt.Sprintf("%s attacks your %s tower with %s!\n", currentPlayer.Username, towerType, troopName)
		if targetTower.IsAlive {
			opponentResult += fmt.Sprintf("Your %s tower has %d HP left.\n", towerType, targetTower.Stats.HP)
		} else {
			opponentResult += fmt.Sprintf("Your %s tower is destroyed!\n", towerType)
		}

		currentPlayer.Conn.Write([]byte(result))
		opponent.Conn.Write([]byte(opponentResult))
		return destroyed, result
	}
}
func isGameOver(player *Player) bool {
	for _, tower := range player.Towers {
		if tower.Type == KingTower && !tower.IsAlive {
			return true
		}
	}
	return false
}

func isValidAttack(targetTower *Tower, opponent *Player) (bool, string) {
	var guard1Alive, guard2Alive bool

	for _, tower := range opponent.Towers {
		if tower.ID == "G1" && tower.IsAlive {
			guard1Alive = true
		}
		if tower.ID == "G2" && tower.IsAlive {
			guard2Alive = true
		}
	}

	if targetTower.ID == "G2" && guard1Alive {
		return false, "Invalid attack, you must destroy Guard Tower 1 before attacking Guard Tower 2.\n"
	}
	if targetTower.ID == "K" && (guard1Alive || guard2Alive) {
		return false, "Invalid attack, You must destroy both Guard Towers before attacking the King Tower.\n"
	}

	return true, ""
}

func setupPlayerAssets(player *Player) {
	kingStats := TowerStats{HP: 2000, ATK: 500, DEF: 300, CRIT: 0.1, EXP: 200}
	guardStats := TowerStats{HP: 1000, ATK: 300, DEF: 100, CRIT: 0.05, EXP: 100}
	player.Towers = []Tower{
		{
			ID:       "K",
			Type:     KingTower,
			Position: 0,
			Stats:    kingStats,
			IsAlive:  true,
		},
		{
			ID:       "G1",
			Type:     GuardTower,
			Position: 1,
			Stats:    guardStats,
			IsAlive:  true,
		},
		{
			ID:       "G2",
			Type:     GuardTower,
			Position: 2,
			Stats:    guardStats,
			IsAlive:  true,
		},
	}
	troopPool := []Troop{
		{
			ID:    "T1",
			Name:  string(PawnTroop),
			Type:  PawnTroop,
			Stats: TroopStats{HP: 50, ATK: 150, DEF: 100, MANA: 3, EXP: 5},
		},
		{
			ID:    "T2",
			Name:  string(BishopTroop),
			Type:  BishopTroop,
			Stats: TroopStats{HP: 100, ATK: 200, DEF: 150, MANA: 4, EXP: 10},
		},
		{
			ID:    "T3",
			Name:  string(RookTroop),
			Type:  RookTroop,
			Stats: TroopStats{HP: 250, ATK: 200, DEF: 200, MANA: 5, EXP: 25},
		},
		{
			ID:    "T4",
			Name:  string(KnightTroop),
			Type:  KnightTroop,
			Stats: TroopStats{HP: 200, ATK: 300, DEF: 150, MANA: 5, EXP: 25},
		},
		{
			ID:    "T5",
			Name:  string(PrinceTroop),
			Type:  PrinceTroop,
			Stats: TroopStats{HP: 500, ATK: 400, DEF: 300, MANA: 6, EXP: 50},
		},
		{
			ID:    "T6",
			Name:  string(QueenTroop),
			Type:  QueenTroop,
			Stats: TroopStats{MANA: 5, EXP: 30},
		},
	}
	rand.Shuffle(len(troopPool), func(i, j int) {
		troopPool[i], troopPool[j] = troopPool[j], troopPool[i]
	})
	player.Troops = troopPool[:3]
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
