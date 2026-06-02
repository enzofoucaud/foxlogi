package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"foxlogi/internal/guildconfig"
	"foxlogi/internal/request"

	"github.com/bwmarrin/discordgo"
)

const deadlineLayout = "2006-01-02"

// RequestCommand implements the /request slash command (new, additem, list,
// fill, cancel subcommands) for the logistics request board.
type RequestCommand struct {
	repo     request.Repository
	settings guildconfig.Repository
}

// NewRequestCommand creates the /request command backed by repo, routing public
// pings through the guild's configured request channel.
func NewRequestCommand(repo request.Repository, settings guildconfig.Repository) *RequestCommand {
	return &RequestCommand{repo: repo, settings: settings}
}

// Definition describes the /request command and its subcommands.
func (c *RequestCommand) Definition() *discordgo.ApplicationCommand {
	minQty := 1.0
	dmPermission := false
	return &discordgo.ApplicationCommand{
		Name:         "request",
		Description:  "Post and fulfill multi-item logistics requests",
		DMPermission: &dmPermission,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "new",
				Description: "Open a new request for a delivery location",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{
						Name:        "location",
						Description: "Where to deliver (e.g. West Stockpile)",
						Type:        discordgo.ApplicationCommandOptionString,
						Required:    true,
						MaxLength:   100,
					},
					{
						Name:        "priority",
						Description: "Urgency (default medium)",
						Type:        discordgo.ApplicationCommandOptionString,
						Choices: []*discordgo.ApplicationCommandOptionChoice{
							{Name: "High", Value: request.PriorityHigh},
							{Name: "Medium", Value: request.PriorityMedium},
							{Name: "Low", Value: request.PriorityLow},
						},
					},
					{
						Name:        "deadline",
						Description: "Optional deadline, format YYYY-MM-DD",
						Type:        discordgo.ApplicationCommandOptionString,
						MaxLength:   10,
					},
				},
			},
			{
				Name:        "additem",
				Description: "Add an item line to your request",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{Name: "id", Description: "Request id", Type: discordgo.ApplicationCommandOptionInteger, Required: true, MinValue: &minQty, Autocomplete: true},
					{Name: "item", Description: "Item name", Type: discordgo.ApplicationCommandOptionString, Required: true, MaxLength: 100},
					{Name: "quantity", Description: "How many", Type: discordgo.ApplicationCommandOptionInteger, Required: true, MinValue: &minQty, MaxValue: 1000000},
				},
			},
			{
				Name:        "list",
				Description: "List open requests on this server",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
			{
				Name:        "fill",
				Description: "Contribute a quantity to a request's item",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{Name: "id", Description: "Request id", Type: discordgo.ApplicationCommandOptionInteger, Required: true, MinValue: &minQty, Autocomplete: true},
					{Name: "item", Description: "Item name", Type: discordgo.ApplicationCommandOptionString, Required: true, MaxLength: 100, Autocomplete: true},
					{Name: "quantity", Description: "How many you delivered", Type: discordgo.ApplicationCommandOptionInteger, Required: true, MinValue: &minQty, MaxValue: 1000000},
				},
			},
			{
				Name:        "cancel",
				Description: "Cancel your own request",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{Name: "id", Description: "Request id", Type: discordgo.ApplicationCommandOptionInteger, Required: true, MinValue: &minQty, Autocomplete: true},
				},
			},
		},
	}
}

// Handle dispatches to the matching subcommand.
func (c *RequestCommand) Handle(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	if len(data.Options) == 0 {
		return
	}
	sub := data.Options[0]
	switch sub.Name {
	case "new":
		c.handleNew(s, i, sub)
	case "additem":
		c.handleAddItem(s, i, sub)
	case "list":
		c.handleList(s, i)
	case "fill":
		c.handleFill(s, i, sub)
	case "cancel":
		c.handleCancel(s, i, sub)
	}
}

// Autocomplete powers the id and item options of /request: id suggests open
// requests, item suggests the targeted request's actual items — so neither can
// be mistyped.
func (c *RequestCommand) Autocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	if len(data.Options) == 0 {
		c.respondChoices(s, i, nil)
		return
	}
	sub := data.Options[0]
	focused := focusedOption(sub.Options)
	if focused == nil {
		c.respondChoices(s, i, nil)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	switch focused.Name {
	case "id":
		c.autocompleteID(ctx, s, i, sub.Name, optionString(focused))
	case "item":
		if sub.Name == "fill" {
			c.autocompleteItem(ctx, s, i, optionMap(sub.Options), optionString(focused))
		} else {
			c.respondChoices(s, i, nil)
		}
	default:
		c.respondChoices(s, i, nil)
	}
}

// autocompleteID suggests open requests by id. For owner-only subcommands
// (additem, cancel) it lists only the invoking user's requests.
func (c *RequestCommand) autocompleteID(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, subName, prefix string) {
	reqs, err := c.repo.ListOpenByGuild(ctx, i.GuildID)
	if err != nil {
		c.respondChoices(s, i, nil)
		return
	}
	ownerOnly := subName == "additem" || subName == "cancel"
	userID := interactionUserID(i)

	var choices []*discordgo.ApplicationCommandOptionChoice
	for _, r := range reqs {
		if ownerOnly && r.UserID != userID {
			continue
		}
		idStr := strconv.FormatInt(r.ID, 10)
		if prefix != "" && !strings.HasPrefix(idStr, prefix) {
			continue
		}
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
			Name:  clampChoiceName(fmt.Sprintf("#%d — %s (%s)", r.ID, r.Location, strings.ToUpper(r.Priority))),
			Value: r.ID,
		})
		if len(choices) == 25 {
			break
		}
	}
	c.respondChoices(s, i, choices)
}

// autocompleteItem suggests the targeted request's items for /request fill.
func (c *RequestCommand) autocompleteItem(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate, opts map[string]*discordgo.ApplicationCommandInteractionDataOption, prefix string) {
	idOpt, ok := opts["id"]
	if !ok {
		c.respondChoices(s, i, nil)
		return
	}
	req, found, err := c.repo.GetRequest(ctx, i.GuildID, idOpt.IntValue())
	if err != nil || !found {
		c.respondChoices(s, i, nil)
		return
	}

	prefix = strings.ToLower(strings.TrimSpace(prefix))
	var choices []*discordgo.ApplicationCommandOptionChoice
	for _, it := range req.Items {
		if prefix != "" && !strings.Contains(strings.ToLower(it.Item), prefix) {
			continue
		}
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
			Name:  clampChoiceName(fmt.Sprintf("%s (%d/%d)", it.Item, it.Delivered, it.Quantity)),
			Value: it.Item,
		})
		if len(choices) == 25 {
			break
		}
	}
	c.respondChoices(s, i, choices)
}

func (c *RequestCommand) respondChoices(s *discordgo.Session, i *discordgo.InteractionCreate, choices []*discordgo.ApplicationCommandOptionChoice) {
	respondAutocomplete(s, i, choices)
}

func (c *RequestCommand) handleNew(s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	userID := interactionUserID(i)
	if userID == "" {
		replyEphemeral(s, i, "⚠️ Could not identify you — please try again.")
		return
	}
	opts := optionMap(sub.Options)

	location := strings.TrimSpace(opts["location"].StringValue())
	if location == "" {
		replyEphemeral(s, i, "⚠️ Location cannot be empty.")
		return
	}

	priority := request.PriorityMedium
	if p, ok := opts["priority"]; ok {
		priority = p.StringValue()
	}

	var deadline *time.Time
	if d, ok := opts["deadline"]; ok {
		raw := strings.TrimSpace(d.StringValue())
		if raw != "" {
			parsed, err := time.Parse(deadlineLayout, raw)
			if err != nil {
				replyEphemeral(s, i, fmt.Sprintf("⚠️ Invalid deadline %q. Use the format `YYYY-MM-DD`.", raw))
				return
			}
			deadline = &parsed
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	req, err := c.repo.CreateRequest(ctx, request.Request{
		GuildID:   i.GuildID,
		ChannelID: i.ChannelID,
		UserID:    userID,
		Location:  location,
		Priority:  priority,
		Deadline:  deadline,
	})
	if err != nil {
		log.Printf("create request: %v", err)
		replyEphemeral(s, i, "❌ Failed to create the request, please try again.")
		return
	}

	replyEphemeral(s, i, fmt.Sprintf("✅ Request **#%d** opened for **%s** (%s). Add items with `/request additem id:%d …`.",
		req.ID, location, priority, req.ID))

	announce := fmt.Sprintf("📋 **%s** opened request #%d — deliver to **%s** (priority: %s).",
		interactionUserName(i), req.ID, location, priority)
	if deadline != nil {
		announce += fmt.Sprintf(" Due <t:%d:D>.", deadline.Unix())
	}
	c.announce(s, i.GuildID, i.ChannelID, announce)
}

func (c *RequestCommand) handleAddItem(s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	userID := interactionUserID(i)
	opts := optionMap(sub.Options)
	id := opts["id"].IntValue()
	item := strings.TrimSpace(opts["item"].StringValue())
	quantity := int(opts["quantity"].IntValue())

	if item == "" {
		replyEphemeral(s, i, "⚠️ Item name cannot be empty.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	req, found, err := c.repo.GetRequest(ctx, i.GuildID, id)
	if err != nil {
		log.Printf("get request: %v", err)
		replyEphemeral(s, i, "❌ Failed to load the request.")
		return
	}
	if !found {
		replyEphemeral(s, i, fmt.Sprintf("⚠️ Request #%d not found on this server.", id))
		return
	}
	if req.UserID != userID {
		replyEphemeral(s, i, "⚠️ Only the requester can add items to this request.")
		return
	}

	if _, err := c.repo.AddItem(ctx, req.ID, item, quantity); err != nil {
		log.Printf("add item: %v", err)
		replyEphemeral(s, i, "❌ Failed to add the item.")
		return
	}
	// Ephemeral only: adding items one by one would otherwise spam the channel
	// while a large request is being built.
	replyEphemeral(s, i, fmt.Sprintf("✅ Added **%d× %s** to request #%d.", quantity, item, req.ID))
}

// announce posts a no-ping activity message to the guild's configured request
// channel, falling back to the origin channel when unset or on send error.
func (c *RequestCommand) announce(s *discordgo.Session, guildID, origin, msg string) {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	target := resolveChannel(ctx, c.settings, guildID, origin,
		func(set guildconfig.Settings) string { return set.RequestChannelID })
	sendWithFallback(s, target, origin, msg)
}

func (c *RequestCommand) handleList(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	reqs, err := c.repo.ListOpenByGuild(ctx, i.GuildID)
	if err != nil {
		log.Printf("list requests: %v", err)
		replyEphemeral(s, i, "❌ Failed to fetch requests.")
		return
	}
	if len(reqs) == 0 {
		replyEphemeral(s, i, "📭 No open requests. Open one with `/request new`.")
		return
	}

	var b strings.Builder
	truncated := 0
	for idx, req := range reqs {
		block := formatRequest(req)
		if b.Len()+len(block) > maxEmbedDescription {
			truncated = len(reqs) - idx
			break
		}
		b.WriteString(block)
	}
	if truncated > 0 {
		fmt.Fprintf(&b, "…and %d more", truncated)
	}

	embed := &discordgo.MessageEmbed{
		Title:       "📦 Open requests",
		Description: b.String(),
		Color:       0x3498DB,
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}},
	}); err != nil {
		log.Printf("respond list: %v", err)
	}
}

func (c *RequestCommand) handleFill(s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	userID := interactionUserID(i)
	opts := optionMap(sub.Options)
	id := opts["id"].IntValue()
	item := strings.TrimSpace(opts["item"].StringValue())
	amount := int(opts["quantity"].IntValue())

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	req, found, err := c.repo.GetRequest(ctx, i.GuildID, id)
	if err != nil {
		log.Printf("get request: %v", err)
		replyEphemeral(s, i, "❌ Failed to load the request.")
		return
	}
	if !found {
		replyEphemeral(s, i, fmt.Sprintf("⚠️ Request #%d not found on this server.", id))
		return
	}

	line, ok, err := c.repo.Fill(ctx, req.ID, item, amount)
	if err != nil {
		log.Printf("fill request: %v", err)
		replyEphemeral(s, i, "❌ Failed to record the contribution.")
		return
	}
	if !ok {
		replyEphemeral(s, i, fmt.Sprintf("⚠️ Request #%d has no item called %q.", id, item))
		return
	}

	replyEphemeral(s, i, fmt.Sprintf("✅ Recorded **+%d %s** on request #%d (%d/%d).",
		amount, line.Item, id, line.Delivered, line.Quantity))

	// Notify the requester in the request's origin channel. Avoid a doubled
	// mention when the requester fills their own request.
	var notice string
	if userID == req.UserID {
		notice = fmt.Sprintf("📦 <@%s> you added **%d %s** to your request #%d — %d/%d.",
			req.UserID, amount, line.Item, id, line.Delivered, line.Quantity)
	} else {
		notice = fmt.Sprintf("📦 <@%s> <@%s> added **%d %s** to request #%d — %d/%d.",
			req.UserID, userID, amount, line.Item, id, line.Delivered, line.Quantity)
	}
	// Routing + completion run after the reply; give them their own budget
	// rather than sharing the ack-window context.
	doneCtx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	target := resolveChannel(doneCtx, c.settings, req.GuildID, req.ChannelID,
		func(set guildconfig.Settings) string { return set.RequestChannelID })
	sendWithFallback(s, target, req.ChannelID, notice)

	fulfilled, err := c.repo.IsFulfilled(doneCtx, req.ID)
	if err != nil {
		log.Printf("is fulfilled: %v", err)
		return
	}
	if !fulfilled {
		return
	}
	// Only the caller that actually removes the request announces it, so a
	// concurrent fill can't double-post the fulfilled message.
	deleted, err := c.repo.DeleteRequest(doneCtx, req.ID)
	if err != nil {
		log.Printf("delete fulfilled request: %v", err)
		return
	}
	if deleted {
		// No ping here: the requester was already pinged by the final
		// contribution message just above.
		sendWithFallback(s, target, req.ChannelID, fmt.Sprintf(
			"🎉 Request #%d for **%s** is fully fulfilled — thanks all!",
			id, req.Location))
	}
}

func (c *RequestCommand) handleCancel(s *discordgo.Session, i *discordgo.InteractionCreate, sub *discordgo.ApplicationCommandInteractionDataOption) {
	userID := interactionUserID(i)
	opts := optionMap(sub.Options)
	id := opts["id"].IntValue()

	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	req, found, err := c.repo.GetRequest(ctx, i.GuildID, id)
	if err != nil {
		log.Printf("get request: %v", err)
		replyEphemeral(s, i, "❌ Failed to load the request.")
		return
	}
	if !found {
		replyEphemeral(s, i, fmt.Sprintf("⚠️ Request #%d not found on this server.", id))
		return
	}
	if req.UserID != userID {
		replyEphemeral(s, i, "⚠️ Only the requester can cancel this request.")
		return
	}

	if _, err := c.repo.DeleteRequest(ctx, req.ID); err != nil {
		log.Printf("cancel request: %v", err)
		replyEphemeral(s, i, "❌ Failed to cancel the request.")
		return
	}
	replyEphemeral(s, i, fmt.Sprintf("🗑️ Request #%d cancelled.", id))

	c.announce(s, req.GuildID, req.ChannelID, fmt.Sprintf("🗑️ Request #%d cancelled.", id))
}

// formatRequest renders one request block for the list embed.
func formatRequest(req request.Request) string {
	var b strings.Builder
	header := fmt.Sprintf("**#%d** `%s` @ %s", req.ID, strings.ToUpper(req.Priority), req.Location)
	if req.Deadline != nil {
		header += fmt.Sprintf(" — due <t:%d:D>", req.Deadline.Unix())
	}
	b.WriteString(header + "\n")
	if len(req.Items) == 0 {
		b.WriteString("  _(no items yet)_\n")
	}
	for _, it := range req.Items {
		mark := ""
		if it.Delivered >= it.Quantity {
			mark = " ✅"
		}
		fmt.Fprintf(&b, "  • %s — %d/%d%s\n", it.Item, it.Delivered, it.Quantity, mark)
	}
	b.WriteString("\n")
	return b.String()
}
