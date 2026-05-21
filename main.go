package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

var members []string
var currentAsker int
var waitingForQuestion string
var waitingForAnswer = make(map[string]bool)
var todaysQuestion string

func main() {
	godotenv.Load()
	token := os.Getenv("TOKEN")

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		panic("Error creating Discord session: " + err.Error())
	}

	dg.Identify.Intents = discordgo.IntentsGuildMembers |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentMessageContent

	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		fmt.Println("Bot is online:", s.State.User.Username)
	})
	dg.AddHandler(messageHandler)

	dg.Open()
	defer dg.Close()

	loadMembers(dg)
	startDailyScheduler(dg)

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, os.Interrupt, syscall.SIGTERM)
	<-sc
}

func startDailyScheduler(s *discordgo.Session) {
	go func() {
		for {
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day(), 1, 18, 30, 0, now.Location())
			if now.After(next) {
				next = next.Add(24 * time.Hour)
			}
			waitDuration := time.Until(next)
			fmt.Printf("Waiting for %v until next scheduled message...\n", waitDuration)

			time.Sleep(waitDuration)
			askNextMember(s)
		}
	}()
}

func loadMembers(s *discordgo.Session) {
	guildId := os.Getenv("GUILD_ID")

	guildMembers, err := s.GuildMembers(guildId, "", 100)
	if err != nil {
		fmt.Println("Error fetching guild members: ", err)
		return
	}

	members = []string{}
	for _, member := range guildMembers {
		if !member.User.Bot {
			members = append(members, member.User.ID)
			fmt.Println(" - Username:", member.User.Username, "| ID:", member.User.ID)
		}
	}
	fmt.Println("Loaded", len(members), "members:")
	for _, id := range members {
		fmt.Println(" -", id)
	}
}

func askNextMember(s *discordgo.Session) {
	if len(members) == 0 {
		log.Println("No members to ask.")
		return
	}
	userID := members[currentAsker]
	waitingForQuestion = userID

	channel, err := s.UserChannelCreate(userID)
	if err != nil {
		log.Printf("Error creating DM channel with %s: %v", userID, err)
		return
	}

	s.ChannelMessageSend(channel.ID, "🎯 It's your turn! Send me today's question and I'll post it anonymously to the group.")
	fmt.Println("Asked", userID, "for today's question.")

	currentAsker = (currentAsker + 1) % len(members)
}

func messageHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	channel, err := s.Channel(m.ChannelID)
	if err != nil || channel.Type != discordgo.ChannelTypeDM {
		return
	}

	channelID := os.Getenv("CHANNEL_ID")
	userID := m.Author.ID

	if waitingForQuestion == userID {
		todaysQuestion = m.Content

		s.ChannelMessageSend(channelID, "**Today's Question:**\n"+todaysQuestion)

		s.ChannelMessageSend(m.ChannelID, "Your question has been posted! Now send me your own answer too.")

		waitingForQuestion = ""
		for _, id := range members {
			waitingForAnswer[id] = true
		}
		return
	}

	if waitingForAnswer[userID] {
		s.ChannelMessageSend(channelID, "**Answer to \""+todaysQuestion+"\":**\n"+m.Content)
		delete(waitingForAnswer, userID)

		s.ChannelMessageSend(m.ChannelID, "Your answer has been posted anonymously!")

		if len(waitingForAnswer) == 0 {
			s.ChannelMessageSend(channelID, "🎉 Everyone has answered today's question!")
		}
		return
	}

	s.ChannelMessageSend(m.ChannelID, "No active question right now. Check back tomorrow!")
}
