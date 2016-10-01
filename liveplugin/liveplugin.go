package liveplugin

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"google.golang.org/api/youtube/v3"

	"github.com/iopred/bruxism"
	"github.com/iopred/discordgo"
)

type livePlugin struct {
	ytLiveChannel            *bruxism.YTLiveChannel
	ChannelToYouTubeChannels map[string]map[string]bool
	youTubeChannelToChannels map[string]map[string]bool
	liveVideoChan            chan *youtube.Video
}

// Name returns the name of the plugin.
func (p *livePlugin) Name() string {
	return "Live"
}

// Load will load plugin state from a byte array.
func (p *livePlugin) Load(bot *bruxism.Bot, service bruxism.Service, data []byte) error {
	if data != nil {
		if err := json.Unmarshal(data, p); err != nil {
			log.Println("Error loading data", err)
		}
	}

	for channel, ytChannels := range p.ChannelToYouTubeChannels {
		for ytChannel := range ytChannels {
			p.monitor(channel, ytChannel)
		}
	}

	go p.Run(bot, service)
	return nil
}

func (p *livePlugin) monitor(channel, ytChannel string) error {
	if p.youTubeChannelToChannels[ytChannel] == nil {
		p.youTubeChannelToChannels[ytChannel] = map[string]bool{}
		err := p.ytLiveChannel.Monitor(ytChannel, p.liveVideoChan)
		if err != nil {
			return err
		}
	}
	if p.ChannelToYouTubeChannels[channel] == nil {
		p.ChannelToYouTubeChannels[channel] = map[string]bool{}
	}
	p.ChannelToYouTubeChannels[channel][ytChannel] = true
	p.youTubeChannelToChannels[ytChannel][channel] = true
	return nil
}

// Run will poll YouTube for channels going live and send messages.
func (p *livePlugin) Run(bot *bruxism.Bot, service bruxism.Service) {
	for {
		v := <-p.liveVideoChan
		for channel := range p.youTubeChannelToChannels[v.Snippet.ChannelId] {
			service.SendMessage(channel, fmt.Sprintf("%s has just gone live! http://gaming.youtube.com/watch?v=%s", v.Snippet.ChannelTitle, v.Id))
		}
	}
}

// Save will save plugin state to a byte array.
func (p *livePlugin) Save() ([]byte, error) {
	return json.Marshal(p)
}

// Help returns a list of help strings that are printed when the user requests them.
func (p *livePlugin) Help(bot *bruxism.Bot, service bruxism.Service, message bruxism.Message, detailed bool) []string {
	if detailed {
		return []string{
			bruxism.CommandHelp(service, "live", "add [youtube channel id]", "Adds a channel to be announced.")[0],
			bruxism.CommandHelp(service, "live", "remove [youtube channel id]", "Removes a channel from being announced.")[0],
			bruxism.CommandHelp(service, "live", "list", "Lists all the channels being announced in this channel.")[0],
		}
	}

	return bruxism.CommandHelp(service, "live", "<add|remove|list> [youtube channel id]", "Announces when a YouTube Channel goes live.")
}

// Message handler.
func (p *livePlugin) Message(bot *bruxism.Bot, service bruxism.Service, message bruxism.Message) {
	defer bruxism.MessageRecover()
	if !service.IsMe(message) {
		messageChannel := message.Channel()

		if bruxism.MatchesCommand(service, "live", message) {
			ticks := ""
			if service.Name() == bruxism.DiscordServiceName {
				ticks = "`"
			}

			_, parts := bruxism.ParseCommand(service, message)

			if len(parts) == 0 {
				service.SendMessage(messageChannel, fmt.Sprintf("Incorrect command. eg: %s%slive [add|remove|list] <%s>%s", ticks, service.CommandPrefix(), "UCGmC0A8mEAPdlELQdP9xJbw", ticks))
			}

			isAuthorized := service.IsModerator(message)

			if service.Name() == bruxism.DiscordServiceName {
				discord := service.(*bruxism.Discord)
				p, err := discord.UserChannelPermissions(message.UserID(), message.Channel())
				if err == nil {
					isAuthorized = isAuthorized || (p&discordgo.PermissionManageRoles != 0) || (p&discordgo.PermissionManageChannels != 0) || (p&discordgo.PermissionManageServer != 0)
				}
			}

			switch parts[0] {
			case "list":
				if !isAuthorized {
					service.SendMessage(messageChannel, "I'm sorry, you must be the channel owner to list live announcements.")
					return
				}
				list := []string{}
				for ytChannel := range p.ChannelToYouTubeChannels[messageChannel] {
					list = append(list, fmt.Sprintf("%s (%s)", p.ytLiveChannel.ChannelName(ytChannel), ytChannel))
				}
				if len(list) == 0 {
					service.SendMessage(messageChannel, "No Channels are being announced.")
				} else {
					service.SendMessage(messageChannel, fmt.Sprintf("Currently announcing: %s", strings.Join(list, ",")))
				}
			case "add":
				if !isAuthorized {
					service.SendMessage(messageChannel, "I'm sorry, you must be the channel owner to add live announcements.")
					return
				}
				if len(parts) != 2 || len(parts[1]) != 24 {
					service.SendMessage(messageChannel, fmt.Sprintf("Incorrect Channel ID. eg: %s%slive %s %s%s", ticks, service.CommandPrefix(), parts[0], "UCGmC0A8mEAPdlELQdP9xJbw", ticks))
					return
				}
				err := p.monitor(messageChannel, parts[1])
				if err != nil {
					service.SendMessage(messageChannel, fmt.Sprintf("Could not add Channel ID. %s", err))
					return
				}
				service.SendMessage(messageChannel, fmt.Sprintf("Messages will be sent here when %s goes live.", p.ytLiveChannel.ChannelName(parts[1])))
			case "remove":
				if !isAuthorized {
					service.SendMessage(messageChannel, "I'm sorry, you must be the channel owner to remove live announcements.")
					return
				}
				if len(parts) != 2 || len(parts[1]) != 24 {
					service.SendMessage(messageChannel, fmt.Sprintf("Incorrect Channel ID. eg: %s%slive %s %s%s", ticks, service.CommandPrefix(), parts[0], "UCGmC0A8mEAPdlELQdP9xJbw", ticks))
					return
				}
				delete(p.ChannelToYouTubeChannels[messageChannel], parts[1])
				delete(p.youTubeChannelToChannels[parts[1]], messageChannel)
				service.SendMessage(messageChannel, fmt.Sprintf("Messages will no longer be sent here when %s goes live.", p.ytLiveChannel.ChannelName(parts[1])))
			}
		}
	}
}

// New will create a new live plugin.
func New(ytLiveChannel *bruxism.YTLiveChannel) bruxism.Plugin {
	return &livePlugin{
		ytLiveChannel:            ytLiveChannel,
		ChannelToYouTubeChannels: map[string]map[string]bool{},
		youTubeChannelToChannels: map[string]map[string]bool{},
		liveVideoChan:            make(chan *youtube.Video),
	}
}
