package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net"
	"os"
	. "project/dto"
	"strings"
	"sync"
	"time"
)

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
func countDestroyedTowers(player *Player) int {
	count := 0
	for _, t := range player.Towers {
		if !t.IsAlive {
			count++
		}
	}
	return count
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
			newPlayer := Player{Username: user.Username, Conn: conn, User: &user}
			players <- newPlayer
			break
		}
	}

	writer.WriteString(fmt.Sprintf("Welcome, %s (Level %d)! You're now connected.\n", user.Fullname, user.Level))
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
	gameOver := make(chan bool)
	mutex := &sync.Mutex{}

	game := GameState{
		Player1: &player1,
		Player2: &player2,
	}

	setupPlayerAssets(game.Player1)
	setupPlayerAssets(game.Player2)
	sendTroopPool(&player1)
	sendTroopPool(&player2)

	player1.Conn.Write([]byte("Game started! You are Player 1.\n"))
	player2.Conn.Write([]byte("Game started! You are Player 2.\n"))

	go listenForMoves(game.Player1, game.Player2, gameOver, mutex)
	go listenForMoves(game.Player2, game.Player1, gameOver, mutex)

	// Timer logic: run for 3 minutes
	timer := time.NewTimer(3 * time.Minute)

	select {
	case <-gameOver:
		saveUsers([]User{*game.Player1.User, *game.Player2.User}, "users.json")
		// A King Tower was destroyed, already handled
		return
	case <-timer.C:
		// Time's up, determine winner
		destroyedByP1 := countDestroyedTowers(game.Player2)
		destroyedByP2 := countDestroyedTowers(game.Player1)

		if destroyedByP1 > destroyedByP2 {
			gainExp(game.Player1.User, 30, game.Player1)
			gainExp(game.Player2.User, 0, game.Player2)
			player1.Conn.Write([]byte("Time's up! You win by destroying more towers.\n"))
			player2.Conn.Write([]byte("Time's up! You lose.\n"))
		} else if destroyedByP2 > destroyedByP1 {
			gainExp(game.Player1.User, 0, game.Player1)
			gainExp(game.Player2.User, 30, game.Player2)
			player2.Conn.Write([]byte("Time's up! You win by destroying more towers.\n"))
			player1.Conn.Write([]byte("Time's up! You lose.\n"))
		} else {
			player1.Conn.Write([]byte("Time's up! It's a draw.\n"))
			player2.Conn.Write([]byte("Time's up! It's a draw.\n"))
		}
		saveUsers([]User{*game.Player1.User, *game.Player2.User}, "users.json")
	}
}

func listenForMoves(currentPlayer *Player, opponent *Player, gameOver chan bool, mutex *sync.Mutex) {
	reader := bufio.NewReader(currentPlayer.Conn)

	for {
		currentPlayer.Conn.Write([]byte("Type your move (e.g., Pawn G1):\n"))
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("Connection lost with %s\n", currentPlayer.Username)
			gameOver <- true
			return
		}
		input = strings.TrimSpace(input)
		if input == "exit" {
			currentPlayer.Conn.Write([]byte("You exited the game.\n"))
			opponent.Conn.Write([]byte("Opponent exited. Game over.\n"))
			gameOver <- true
			return
		}

		mutex.Lock()
		applyMove(currentPlayer, opponent, input)
		mutex.Unlock()

		// Check if opponent’s King Tower is destroyed
		if isGameOver(opponent, gameOver) {
			currentPlayer.Conn.Write([]byte("You destroyed the King Tower! You win!\n"))
			opponent.Conn.Write([]byte("Your King Tower was destroyed. You lose.\n"))
			return
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
			leveledUp := gainExp(currentPlayer.User, targetTower.Stats.EXP, currentPlayer)
			currentPlayer.Conn.Write([]byte(fmt.Sprintf("You gain %d exp", targetTower.Stats.EXP)))
			if leveledUp {
				currentPlayer.Conn.Write([]byte("Level up! Your stats increased by 10%!\n"))
			}
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
func isGameOver(player *Player, gameOver chan bool) bool {
	for _, tower := range player.Towers {
		if tower.Type == KingTower && !tower.IsAlive {
			gameOver <- true
			return true
		}
	}
	return false
}

func gainExp(user *User, amount int, player *Player) bool {
	leveledUp := false
	user.EXP += amount
	baseExp := 100
	for {
		requiredExpToLevelUp := int(float64(baseExp) * math.Pow(1.1, float64(user.Level)))
		if user.EXP >= requiredExpToLevelUp {
			user.EXP -= requiredExpToLevelUp
			user.Level++
			leveledUp = true
			applyLeveledUpBuff(player)
		} else {
			break
		}
	}
	return leveledUp
}

func applyLeveledUpBuff(player *Player) {
	for i := range player.Towers {
		tower := &player.Towers[i]
		tower.Stats.HP = int(float64(tower.Stats.HP) * 1.1)
		tower.Stats.ATK = int(float64(tower.Stats.ATK) * 1.1)
		tower.Stats.DEF = int(float64(tower.Stats.DEF) * 1.1)
		tower.Stats.EXP = int(float64(tower.Stats.EXP) * 1.1)
	}
	for i := range player.Troops {
		tr := &player.Troops[i]
		tr.Stats.HP = int(float64(tr.Stats.HP) * 1.1)
		tr.Stats.ATK = int(float64(tr.Stats.ATK) * 1.1)
		tr.Stats.DEF = int(float64(tr.Stats.DEF) * 1.1)
		tr.Stats.MANA = int(float64(tr.Stats.MANA) * 1.1)
		tr.Stats.EXP = int(float64(tr.Stats.EXP) * 1.1)
	}
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
	level := player.User.Level
	multiplier := math.Pow(1.1, float64(level))
	kingStats := TowerStats{
		HP:   int(2000 * multiplier),
		ATK:  int(500 * multiplier),
		DEF:  int(300 * multiplier),
		CRIT: 0.1, // optional: keep constant or increase too?
		EXP:  int(200 * multiplier),
	}

	guardStats := TowerStats{
		HP:   int(1000 * multiplier),
		ATK:  int(300 * multiplier),
		DEF:  int(100 * multiplier),
		CRIT: 0.05,
		EXP:  int(100 * multiplier),
	}
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
	for i := range troopPool {
		t := &troopPool[i]
		t.Stats.HP = int(float64(t.Stats.HP) * multiplier)
		t.Stats.ATK = int(float64(t.Stats.ATK) * multiplier)
		t.Stats.DEF = int(float64(t.Stats.DEF) * multiplier)
		t.Stats.MANA = int(float64(t.Stats.MANA) * multiplier)
		t.Stats.EXP = int(float64(t.Stats.EXP) * multiplier)
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
