package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/bwmarrin/discordgo"
)

type Config struct {
	BotID     int64
	BotToken  string
	BotSecret string
	GuildIDs  []string
}

func main() {
	// Load and decode config
	b, err := ioutil.ReadFile("./config.toml")
	if err != nil {
		fmt.Printf("Unable to load config file: %s\n", err.Error())
		os.Exit(1)
	}
	var cfg Config
	if _, err := toml.Decode(string(b), &cfg); err != nil {
		fmt.Printf("Unable to decode config file: %s\n", err.Error())
		os.Exit(1)
	}

	// Setup Discord API
	connectionString := fmt.Sprintf("Bot %s", cfg.BotToken)
	discord, discErr := discordgo.New(connectionString)
	if discErr != nil {
		fmt.Println("Error creating Discord Session :", discErr.Error())
		os.Exit(2)
	}

	var timeGuilds = make(map[string]map[string]*discordgo.Channel)

	for _, guildID := range cfg.GuildIDs {
		// Get all of the Discord channels in the server
		chs, err := discord.GuildChannels(guildID)
		if err != nil {
			fmt.Println("Cannot read channels of guild")
			os.Exit(1)
		}

		// Determine if the existing voice channel exists for time keeping
		timeGuilds[guildID] = map[string]*discordgo.Channel{
			"UTC":  nil,
			"PST":  nil,
			"EST":  nil,
			"AEST": nil,
		}

		for _, chv := range chs {
			if chv.Type != discordgo.ChannelTypeGuildVoice {
				continue
			}
			// Find the channel based on whether or not it has a clock at the start of the channel name
			chNameParts := strings.Split(chv.Name, " ")
			if len(chNameParts) > 2 { // Change this to 3 if using clock faces
				// clockFace := chNameParts[0]
				// tz := chNameParts[2]
				tz := chNameParts[1]
				mapTZ := ""

				switch tz {
				case "PST", "PDT":
					mapTZ = "PST"
				case "EST", "EDT":
					mapTZ = "EST"
				case "AEST", "AEDT":
					mapTZ = "AEST"
				case "UTC":
					mapTZ = "UTC"
				default:
					continue
				}
				timeGuilds[guildID][mapTZ] = chv
			}
		}

		// If the channel doesn't exist, create it
		for k := range timeGuilds[guildID] {
			if timeGuilds[guildID][k] == nil {
				format := makeChannelName(k)
				timeGuilds[guildID][k], err = discord.GuildChannelCreate(guildID, format, discordgo.ChannelTypeGuildVoice)
				if err != nil {
					fmt.Printf("Channel Make Error: %s\n", err.Error())
					os.Exit(1)
				}
			}
		}
	}

	// Check the time every 5 seconds
	ticker := time.NewTicker(5 * time.Second)
	quit := make(chan bool)
	go func() {
		for {
			select {
			case <-ticker.C:
				// Only update every 5 minutes
				if time.Now().Minute()%5 == 0 {
					for _, timeChannels := range timeGuilds {
						for k := range timeChannels {
							if timeChannels[k] == nil {
								continue
							}
							channelNameFormat := makeChannelName(k)
							// Only attempt to update the channel name if the new name channelNameFormat doesn't match the current one
							// This is so that we don't keep trying to update 5:00 to 5:00 for example, since there would
							// theoretically be 11 attempts to change the name in the minute and Discord channels have a rate
							// limit of 2 updates per 10 minutes
							if timeChannels[k].Name != channelNameFormat {
								format := &discordgo.ChannelEdit{
									Name: channelNameFormat,
								}
								timeChannels[k], err = discord.ChannelEdit(timeChannels[k].ID, format)
								if err != nil {
									fmt.Printf("Channel Edit Error: %s\n", err.Error())
								}
							}
						}
					}
				}
			case <-quit:
				ticker.Stop()
				fmt.Println("Stopping channel updates")
				return
			}
		}
	}()

	// Create the websocket connection
	discErr = discord.Open()
	if discErr != nil {
		fmt.Println(discErr)
		os.Exit(3)
	}

	// Keep the connection open until interrupted
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, syscall.SIGTERM)
	<-sc

	// Signal the ticker to stop and close Discord
	quit <- true
	discord.Close()
}

func makeChannelName(location string) string {
	utcTime := time.Now().UTC()
	var locTime time.Time

	switch location {
	case "PST":
		locTime = localizeTime(utcTime, "America/Los_Angeles")
	case "EST":
		locTime = localizeTime(utcTime, "America/New_York")
	case "AEST":
		locTime = localizeTime(utcTime, "Australia/Melbourne")
	default:
		locTime = utcTime
	}

	timeStrName := locTime.Format("15:04 MST | Mon Jan 02")

	return timeStrName
}

func localizeTime(timeUTC time.Time, location string) time.Time {
	loc, err := time.LoadLocation(location)
	if err != nil {
		fmt.Printf("Unable to load location: %s\n", err.Error())
	}

	return timeUTC.In(loc)
}
