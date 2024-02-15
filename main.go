package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	"github.com/mtslzr/pokeapi-go"
	"github.com/mtslzr/pokeapi-go/structs"
)

type PokeData struct {
	Pokemon        structs.Pokemon
	PokemonSpecies structs.PokemonSpecies
	EvolutionChain structs.EvolutionChain
}

var (
	Token        string
	Guess        map[string]PokeData
	prefix       string = "!joy"
	maxPokemonId int    = 476 // NOTE: Gen 1 - 4
	dbPath       string = "database.db"
)

func init() {
	flag.StringVar(&Token, "t", "", "Bot Token")
	flag.StringVar(&dbPath, "dbPath", dbPath, "Path to database")
	flag.Parse()
}

func main() {
	err := godotenv.Load()
	if err != nil {
		slog.Info("Not loading .env file", err)
	}

	envToken := os.Getenv("DISCORD_TOKEN")

	if Token == "" {
		Token = envToken
	}

	if Token == "" {
		panic("Token is required")
	}

	Guess = make(map[string]PokeData)

	err = initDatabase(dbPath)
	defer db.Close()
	if err != nil {
		slog.Error("can' init database", "error", err)
	}

	discord, err := discordgo.New("Bot " + Token)
	if err != nil {
		slog.Error("error creating Discord session: %s\n", "error", err)
		return
	}
	defer discord.Close()

	discord.AddHandler(onMessageCreate)

	err = discord.Open()
	if err != nil {
		slog.Error("error opening connection: %s\n", "error", err)
		return
	}

	slog.Info("Bot is now running.  Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
}

func onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot || !strings.HasPrefix(m.Content, prefix) {
		return
	}

	if err := s.ChannelTyping(m.ChannelID); err != nil {
		slog.Error("can't send typing", "guildId", m.GuildID, "channelId", m.ChannelID, "error", err)
	}

	command := strings.TrimPrefix(m.Content, prefix)
	command = strings.Trim(command, "")
	slog.Info("command", "command", command)

	if _, ok := Guess[m.GuildID]; !ok {
		randomPokemon(m.GuildID)
	}

	var err error

	answer := Guess[m.GuildID]
	name := strings.ToLower(strings.TrimSpace(command))

	if name == "giveup" || name == "give up" {
		_, err = s.ChannelMessageSend(m.ChannelID, answer.Pokemon.Name)
		if err != nil {
			slog.Error("can't send give up message", "error", err)
		}

		randomPokemon(m.GuildID)
	} else if name == answer.Pokemon.Name {
		_, description := compare(answer, answer)
		_, err = s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
			Type:        "rich",
			Title:       answer.Pokemon.Name,
			Description: description,
			Color:       0x00FF00,
			Thumbnail: &discordgo.MessageEmbedThumbnail{
				URL: answer.Pokemon.Sprites.FrontDefault,
			},
		})
		if err != nil {
			slog.Error("can't send message", "guildId", m.GuildID, "error", err)
		}

		randomPokemon(m.GuildID)
		return
	}

	guessData, err := findPokemon(name, false)
	if err != nil {
		slog.Error("can't get pokemon", "guildId", m.GuildID, "pokemonName", name, "error", err)

		search, err := pokeapi.Search("pokemon", name)
		if err != nil {
			slog.Error("can't search pokemon", "guildId", m.GuildID, "pokemonName", name, "error", err)
		}
		similarName := []string{}
		for _, result := range search.Results {
			similarName = append(similarName, result.Name)
		}

		s.ChannelMessageSend(m.ChannelID, "Not found pokemon name: "+name+"\nSimilar pokemon name: "+strings.Join(similarName, ", "))
		return
	}

	_, description := compare(answer, guessData)

	_, err = s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Type:        "rich",
		Title:       guessData.Pokemon.Name,
		Description: description,
		Color:       0xFFFFFF,
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: guessData.Pokemon.Sprites.FrontDefault,
		},
	})
	if err != nil {
		slog.Error("can't send message", "guildId", m.GuildID, "error", err)
	}
}

func compare(answer PokeData, guess PokeData) (bool, string) {
	correct := 0
	msg := ""
	nameLabel := "**Name**: "
	nameLabelSuffix := ""
	if strings.Contains(answer.Pokemon.Name, "-") {
		nameLabelSuffix = " (Current Pokemon's name have gender or forms in name) "
	}
	if answer.Pokemon.Name == guess.Pokemon.Name {
		msg += ":green_square:" + nameLabel + guess.Pokemon.Name + nameLabelSuffix + "\n"
		correct += 1
	} else {
		msg += ":red_square:" + nameLabel + guess.Pokemon.Name + nameLabelSuffix + "\n"
	}

	typeCorrect := 0
	for _, ansType := range answer.Pokemon.Types {
		match := false
		for _, guessType := range guess.Pokemon.Types {
			if ansType.Type.Name == guessType.Type.Name {
				match = true
				break
			}
		}

		if match {
			typeCorrect += 1
		}
	}

	typeLabel := "**Type(s)**: "
	if typeCorrect == len(answer.Pokemon.Types) && len(answer.Pokemon.Types) == len(guess.Pokemon.Types) {
		msg += ":green_square:" + typeLabel
		for i, guessType := range guess.Pokemon.Types {
			if i != 0 {
				msg += ", "
			}
			msg += guessType.Type.Name
		}
		msg += "\n"
		correct += 1
	} else if typeCorrect == 0 {
		msg += ":red_square:" + typeLabel
		for i, guessType := range guess.Pokemon.Types {
			if i != 0 {
				msg += ", "
			}
			msg += guessType.Type.Name
		}
		msg += "\n"
	} else {
		msg += ":yellow_square:" + typeLabel
		for i, guessType := range guess.Pokemon.Types {
			if i != 0 {
				msg += ", "
			}
			msg += guessType.Type.Name
		}
		msg += "\n"
	}

	colorLabel := "**Main Color**: "
	if answer.PokemonSpecies.Color.Name == guess.PokemonSpecies.Color.Name {
		msg += ":green_square:" + colorLabel + guess.PokemonSpecies.Color.Name + "\n"
		correct += 1
	} else {
		msg += ":red_square:" + colorLabel + guess.PokemonSpecies.Color.Name + "\n"
	}

	eggGroupLabel := "**Egg groups**: "
	eggGroupsCorrect := 0
	for _, ansGroup := range answer.PokemonSpecies.EggGroups {
		match := false
		for _, guessGroup := range guess.PokemonSpecies.EggGroups {
			if ansGroup.Name == guessGroup.Name {
				match = true
				break
			}
		}

		if match {
			eggGroupsCorrect += 1
		} else {
			// Incorrect
			eggGroupsCorrect = -1
			break
		}
	}
	if eggGroupsCorrect == len(answer.PokemonSpecies.EggGroups) {
		msg += ":green_square:" + eggGroupLabel
		for i, guessGroup := range guess.PokemonSpecies.EggGroups {
			if i != 0 {
				msg += ", "
			}
			msg += guessGroup.Name
		}
		msg += "\n"
		correct += 1
	} else if eggGroupsCorrect == -1 {
		msg += ":red_square:" + eggGroupLabel
		for i, guessGroup := range guess.PokemonSpecies.EggGroups {
			if i != 0 {
				msg += ", "
			}
			msg += guessGroup.Name
		}
		msg += "\n"
	} else {
		msg += ":yellow_square:" + eggGroupLabel
		for i, guessGroup := range guess.PokemonSpecies.EggGroups {
			if i != 0 {
				msg += ", "
			}
			msg += guessGroup.Name
		}
		msg += "\n"
	}

	shapeLabel := "**Evolution stage**:\t\t"
	answerStage, err := getEvolutionStage(answer.Pokemon, answer.EvolutionChain)
	if err != nil {
		slog.Error("can't get evolution stage", "error", err)
	}
	guessStage, err := getEvolutionStage(guess.Pokemon, guess.EvolutionChain)
	if err != nil {
		slog.Error("can't get evolution stage", "error", err)
	}
	if answerStage == guessStage {
		msg += ":green_square:" + shapeLabel + fmt.Sprint(guessStage) + "\n"
		correct += 1
	} else {
		msg += ":red_square:" + shapeLabel + fmt.Sprint(guessStage) + "\n"
	}

	habitatLabel := "**Habitat**:\t\t"
	if answer.PokemonSpecies.Habitat.Name == guess.PokemonSpecies.Habitat.Name {
		msg += ":green_square:" + habitatLabel + guess.PokemonSpecies.Habitat.Name + "\n"
		correct += 1
	} else {
		msg += ":red_square:" + habitatLabel + guess.PokemonSpecies.Habitat.Name + "\n"
	}

	genLabel := "**Generation**: "
	gen, genNum := compareGeneration(answer, guess)
	if gen == 0 {
		msg += ":green_square:" + genLabel + fmt.Sprintf("%v", genNum) + "\n"
		correct += 1
	} else {
		msg += ":red_square:" + genLabel + fmt.Sprintf("%v", genNum) + "\n"
	}

	return correct == 7, msg
}

func compareGeneration(answer PokeData, guess PokeData) (int, int) {
	ansGen, _ := strconv.Atoi(strings.ReplaceAll(strings.ReplaceAll(answer.PokemonSpecies.Generation.URL, "https://pokeapi.co/api/v2/generation/", ""), "/", ""))
	guessGen, _ := strconv.Atoi(strings.ReplaceAll(strings.ReplaceAll(guess.PokemonSpecies.Generation.URL, "https://pokeapi.co/api/v2/generation/", ""), "/", ""))

	if ansGen == guessGen {
		return 0, guessGen
	} else if ansGen > guessGen {
		return 1, guessGen
	} else {
		return -1, guessGen
	}
}

func randomPokemon(guidId string) {
	for {
		slog.Info("random pokemon", "guildId", guidId)
		id := rand.Intn(maxPokemonId-1) + 1

		data, err := findPokemon(fmt.Sprintf("%v", id), true)
		if err != nil {
			slog.Error("can't find pokemon from random", "guildId", guidId, "pokemonId", id, "error", err)
			continue
		}

		Guess[guidId] = data
		slog.Info("finish random pokemon", "guildId", guidId, "pokemonId", data.Pokemon.ID, "pokemonName", data.Pokemon.Name)
		break
	}
}

func findPokemon(id string, byId bool) (PokeData, error) {
	ok, cacheData, err := loadPokemon(dbPath, id, byId)
	if err != nil {
		slog.Error("can't load pokemon", "pokemonId", id, "error", err)
	}

	if ok {
		slog.Info("load pokemon from cache", "pokemonId", cacheData.Pokemon.ID, "pokemonName", cacheData.Pokemon.Name)
		return cacheData, nil
	}

	pokemon, err := pokeapi.Pokemon(id)
	if err != nil {
		slog.Error("can't find pokemon", "id", id, "error", err)
		return PokeData{}, err
	}

	pokemonSpecies, err := pokeapi.PokemonSpecies(pokemon.Species.Name)
	if err != nil {
		slog.Error("can't get pokemon species", "id", id, "error", err)
		return PokeData{}, err
	}

	chainId := strings.ReplaceAll(strings.ReplaceAll(pokemonSpecies.EvolutionChain.URL, "https://pokeapi.co/api/v2/evolution-chain/", ""), "/", "")
	evolutionChain, err := pokeapi.EvolutionChain(chainId)
	if err != nil {
		slog.Error("can't get evolution chain", "id", id, "error", err)
		return PokeData{}, err
	}

	data := PokeData{pokemon, pokemonSpecies, evolutionChain}

	go func() {
		if err := writePokemon(dbPath, data); err != nil {
			slog.Error("can't write pokemon", "id", id, "error", err)
		}
	}()

	return data, nil
}

func getEvolutionStage(pokemon structs.Pokemon, evolutionChain structs.EvolutionChain) (int, error) {
	chain := evolutionChain.Chain
	name := strings.Split(pokemon.Name, "-")[0]
	if chain.Species.Name == name {
		return 1, nil
	}

	if len(chain.EvolvesTo) == 0 {
		return 0, errors.New("can't find evolution stage")
	}

	if chain.EvolvesTo[0].Species.Name == name {
		return 2, nil
	}

	if len(chain.EvolvesTo[0].EvolvesTo) == 0 {
		return 0, errors.New("can't find evolution stage")
	}

	if chain.EvolvesTo[0].EvolvesTo[0].Species.Name == name {
		return 3, nil
	}

	return 0, errors.New("can't find evolution stage")
}
