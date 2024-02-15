package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

var db *sql.DB

var dbCacheTime = 24 * time.Hour * 7

func initDatabase(dbPath string) error {
	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}

	if _, err := db.ExecContext(context.Background(),
		`CREATE TABLE IF NOT EXISTS pokemon(
			id INTEGER,
			name TEXT,
			data BLOB,
			update_at DATETIME,
			PRIMARY KEY(name)
		)`); err != nil {
		return err
	}

	return nil
}

func writePokemon(dbPath string, pokemon PokeData) error {

	data, err := json.Marshal(pokemon)
	if err != nil {
		return err
	}

	_, err = db.Exec("INSERT OR REPLACE INTO pokemon (id, name, data, update_at) VALUES (?, ?, ?, ?)", pokemon.Pokemon.ID, pokemon.Pokemon.Name, data, time.Now())
	if err != nil {
		return err
	}

	return nil
}

type SavedPokemonData struct {
	Id       int             `json:"id"`
	Name     string          `json:"name"`
	Data     json.RawMessage `json:"data"`
	UpdateAt time.Time       `json:"update_at"`
}

func loadPokemon(dbPath string, name string, byId bool) (bool, PokeData, error) {

	var rows *sql.Rows
	var err error
	if byId {
		rows, err = db.Query("SELECT * FROM pokemon WHERE id = ?", name)
	} else {
		rows, err = db.Query("SELECT * FROM pokemon WHERE name = ?", name)
	}
	if err != nil {
		return false, PokeData{}, err
	}

	var pokemons []SavedPokemonData
	for rows.Next() {
		var r SavedPokemonData
		var data []byte
		if err := rows.Scan(&r.Id, &r.Name, &data, &r.UpdateAt); err != nil {
			return false, PokeData{}, err
		}

		r.Data = json.RawMessage(data)
		pokemons = append(pokemons, r)
	}

	if len(pokemons) == 0 {
		return false, PokeData{}, fmt.Errorf("not found pokemon")
	}

	if pokemons[0].UpdateAt.Add(dbCacheTime).Before(time.Now()) {
		return false, PokeData{}, fmt.Errorf("cache expired")
	}

	var pokemon PokeData
	if err := json.Unmarshal(pokemons[0].Data, &pokemon); err != nil {
		return false, PokeData{}, err
	}

	return true, pokemon, nil
}
