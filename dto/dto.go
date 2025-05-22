package dto

import "net"

type Player struct {
	Username string
	Conn     net.Conn
	Towers   []Tower
	Troops   []Troop
	User     *User
}
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
	Username string
	Password string
	Fullname string
	EXP      int
	Level    int
}
