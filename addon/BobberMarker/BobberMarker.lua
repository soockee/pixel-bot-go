-- Bobber Marker draws solid magenta corner brackets around the player's own
-- fishing bobber while the player channels Fishing, for Go Fish to
-- find on screen. The bot clicks the centre of the box and watches the box for
-- the splash, so the box is centred on the bobber and drawn in exact pixels.
--
-- Addons cannot ask where a world object is on screen, but the game's soft
-- targeting (the "softinteract" unit) can give the bobber a nameplate, and the
-- brackets hang on that nameplate. The same approach as ForeverLootSparkles and
-- NaowhForever's Glimmer, which draws softer brackets that change colour with
-- the water behind them.

local SOFT = "softinteract";
local BOBBER = 35591; -- game object ID of the fishing bobber, the same in every language
local GUID_OBJECT = "^GameObject%-%d+%-%d+%-%d+%-%d+%-(%d+)%-";

-- In screen pixels. Go Fish's locator expects arms of at least 10
-- pixels and a box side between 30 and 400 pixels.
local BOX = 96;
local ARM = 20;
local THICKNESS = 3;
local DEFAULT_Y = -40; -- the bobber sits this far below its nameplate's centre
local OFFSET_RADIUS = 2;
local COLOR = { 1, 0, 1 }; -- opaque magenta: exact over any background, and rare in the world

local POLL = 0.1; -- seconds between checks while fishing

-- Soft targeting must be on for the bobber to become the soft target. These
-- are set once, the first time you fish, and stay as you set them afterwards.
local NEEDED = {
	SoftTargetInteract = "3",            -- the game's "Enable Interact Key"
	SoftTargetIconGameObject = "1",      -- world objects can get a nameplate
	SoftTargetInteractOnlyInRange = "0", -- the bobber is often beyond interact range
	SoftTargetInteractRange = "60",      -- the game still caps it
};
-- The nameplate itself is only on while you fish, then goes back to your value.
local PLATE = "SoftTargetNameplateInteract";

local db;
local fishing = false;
local offsetX, offsetY = 0, 0;
local plateBefore; -- your SoftTargetNameplateInteract while the marker had it on
local anchor;      -- the nameplate the marker hangs on

local function Readable(value)
	return value ~= nil and not (issecretvalue and issecretvalue(value));
end

local function GetCVarValue(name)
	local ok, value = pcall(C_CVar.GetCVar, name);
	if ok and Readable(value) then
		return value;
	end
end

-- Writes only real changes: every write runs other addons' CVAR_UPDATE handlers.
local function SetCVarValue(name, value)
	local current = GetCVarValue(name);
	if current == nil or current == value or InCombatLockdown() then
		return;
	end
	pcall(C_CVar.SetCVar, name, value);
end

---------------------------------------------------------------------------
-- Marker
---------------------------------------------------------------------------

local marker = CreateFrame("Frame");
marker:Hide();
local bars = {};
for i = 1, 8 do
	local bar = marker:CreateTexture(nil, "OVERLAY");
	bar:SetColorTexture(COLOR[1], COLOR[2], COLOR[3], 1);
	if bar.SetSnapToPixelGrid then
		bar:SetSnapToPixelGrid(true);
		bar:SetTexelSnappingBias(0);
	end
	bars[i] = bar;
end

-- UI units per screen pixel for a frame, whatever the UI scale.
local function PixelSize(frame)
	local _, height = GetPhysicalScreenSize();
	return 768 / height / frame:GetEffectiveScale();
end

-- Sizes the box and its brackets in screen pixels. Each corner gets a
-- horizontal and a vertical bar that meet in the corner.
local playerSignal = CreateFrame("Frame", nil, UIParent);
playerSignal:SetFrameStrata("TOOLTIP");
playerSignal:Hide();
local signalBlocks = {};
local previousPlayer;
local playerSequence = 0;
for i, rgb in ipairs({ { 0, 1, 1 }, { 1, 1, 0 }, { 1, 0, 1 }, { 0, 0, 1 }, { 0, 0, 0 } }) do
    local block = playerSignal:CreateTexture(nil, "OVERLAY");
    block:SetColorTexture(rgb[1], rgb[2], rgb[3], 1);
    signalBlocks[i] = block;
end

local function UpdatePlayerSignal()
    local key = BobberMarkerKeys and BobberMarkerKeys.targetPlayer;
    if not key or key == "" or not fishing or InCombatLockdown() or
        GetBindingAction(key, true) ~= "CLICK BobberMarkerTargetPlayer:LeftButton" then
        return playerSignal:Hide();
    end
    local ok, guid = pcall(function()
        local player = UnitIsPlayer("softfriend");
        local self = UnitIsUnit("softfriend", "player");
        local selected = UnitIsUnit("softfriend", "target");
        if issecretvalue and (issecretvalue(player) or issecretvalue(self) or issecretvalue(selected)) then
            return false;
        end
        if player and not self and not selected then
            local id = UnitGUID("softfriend");
            if Readable(id) and type(id) == "string" then
                return id;
            end
        end
    end);
    if not ok or not guid then
        return playerSignal:Hide();
    end
    if guid ~= previousPlayer then
        previousPlayer = guid;
        playerSequence = playerSequence % 255 + 1;
        signalBlocks[5]:SetColorTexture(playerSequence / 255, 211 / 255, 77 / 255, 1);
    end
    local px = PixelSize(playerSignal);
    playerSignal:ClearAllPoints();
    playerSignal:SetPoint("TOPLEFT", UIParent, "TOPLEFT", 8 * px, -8 * px);
    playerSignal:SetSize(30 * px, 6 * px);
    for i, block in ipairs(signalBlocks) do
        block:SetPoint("TOPLEFT", playerSignal, "TOPLEFT", (i - 1) * 6 * px, 0);
        block:SetSize(6 * px, 6 * px);
    end
    playerSignal:Show();
end

-- Tells Go Fish the player is in the world. Addons do not run on the
-- character screen, so no blocks there means logged out. Without a parent it
-- draws over the interface and stays while Alt+Z hides UIParent; a WorldFrame
-- child would draw under the interface.
local worldSignal = CreateFrame("Frame");
worldSignal:SetFrameStrata("TOOLTIP");
local worldBlocks = {};
for i, rgb in ipairs({ { 1, 0, 1 }, { 0, 1, 0 }, { 1, 0, 1 } }) do
    local block = worldSignal:CreateTexture(nil, "OVERLAY");
    block:SetColorTexture(rgb[1], rgb[2], rgb[3], 1);
    worldBlocks[i] = block;
end

local function LayoutWorldSignal()
    local px = PixelSize(worldSignal);
    worldSignal:ClearAllPoints();
    worldSignal:SetPoint("TOPLEFT", WorldFrame, "TOPLEFT", 8 * px, -16 * px);
    worldSignal:SetSize(18 * px, 6 * px);
    for i, block in ipairs(worldBlocks) do
        block:SetPoint("TOPLEFT", worldSignal, "TOPLEFT", (i - 1) * 6 * px, 0);
        block:SetSize(6 * px, 6 * px);
    end
end

local function Layout()
	local px = PixelSize(marker);
	marker:SetSize(BOX * px, BOX * px);
	local i = 1;
	for _, corner in ipairs({ "TOPLEFT", "TOPRIGHT", "BOTTOMLEFT", "BOTTOMRIGHT" }) do
		for _, size in ipairs({ { ARM, THICKNESS }, { THICKNESS, ARM } }) do
			local bar = bars[i];
			bar:ClearAllPoints();
			bar:SetPoint(corner, marker, corner, 0, 0);
			bar:SetSize(size[1] * px, size[2] * px);
			i = i + 1;
		end
	end
end

local function Release()
	if not anchor then
		return;
	end
	marker:Hide();
	marker:ClearAllPoints();
	marker:SetParent(nil);
	anchor = nil;
end

local function Attach(plate)
	marker:SetParent(plate);
	marker:SetIgnoreParentAlpha(true);
	local ok, level = pcall(plate.GetFrameLevel, plate);
	if ok and Readable(level) then
		marker:SetFrameLevel(level + 10); -- above the nameplate's own parts
	end
	Layout();
	marker:ClearAllPoints();
	local px = PixelSize(marker);
	marker:SetPoint("CENTER", plate, "CENTER", offsetX * px, (db.y + offsetY) * px);
	marker:Show();
	anchor = plate;
end

local function IsBobber(unit)
	local ok, guid = pcall(UnitGUID, unit);
	if not ok or not Readable(guid) or type(guid) ~= "string" then
		return false;
	end
	return tonumber(guid:match(GUID_OBJECT)) == BOBBER;
end

-- Shows the marker while you fish and the soft target is your bobber with a nameplate.
local function Evaluate()
	if not fishing or not IsBobber(SOFT) then
		return Release();
	end
	local ok, plate = pcall(C_NamePlate.GetNamePlateForUnit, SOFT);
	if not ok or not Readable(plate) or not plate then
		return Release();
	end
	if plate ~= anchor then
		Release();
		Attach(plate);
	end
end

---------------------------------------------------------------------------
-- Fishing
---------------------------------------------------------------------------

local poller = CreateFrame("Frame");
poller:Hide();
local sinceCheck = 0;
poller:SetScript("OnUpdate", function(_, elapsed)
	sinceCheck = sinceCheck + elapsed;
	if sinceCheck >= POLL then
		sinceCheck = 0;
		Evaluate();
		UpdatePlayerSignal();
	end
end);

local function StartFishing()
	if not fishing then
		-- Reattaching the same cast's marker must keep the watched box steady.
		offsetX = math.random(-OFFSET_RADIUS, OFFSET_RADIUS);
		offsetY = math.random(-OFFSET_RADIUS, OFFSET_RADIUS);
	end
	fishing = true;
	if not db.prepared and not InCombatLockdown() then
		for name, value in pairs(NEEDED) do
			SetCVarValue(name, value);
		end
		db.prepared = true;
	end
	if plateBefore == nil then
		plateBefore = GetCVarValue(PLATE);
	end
	SetCVarValue(PLATE, "1");
	poller:Show();
	Evaluate();
end

local function StopFishing()
	fishing = false;
	poller:Hide();
	playerSignal:Hide();
	Release();
	if plateBefore ~= nil then
		SetCVarValue(PLATE, plateBefore);
		plateBefore = nil;
	end
end

-- Fishing is a channel, so any channel of yours starts watching; the bobber
-- check decides what is marked. The language of the spell name never matters.
local function IsChanneling()
	local ok, name = pcall(UnitChannelInfo, "player");
	return ok and Readable(name) and name ~= nil;
end

---------------------------------------------------------------------------
-- Keys
---------------------------------------------------------------------------

-- Go Fish presses the Cast key and, when it reels with the Interact key, the
-- Interact key. Its installer writes both into BobberMarkerKeys (Keys.lua,
-- loaded before this file), and this puts them to work in the game so nobody has
-- to: a macro that casts Fishing on the Cast key, and the game's own Interact
-- action on the Interact key. Macros and bindings cannot change in combat, so
-- this waits for it to end. It changes only what differs and never writes to the
-- chat; what went wrong is kept in keyProblems for /bobbermarker to show.

local CAST_MACRO = "Fishing";
local FISHING = 7620;      -- the spell ID of Fishing; its name is looked up, so the macro follows the game's language
local MACRO_ICON = 134400; -- the question mark icon
local INTERACT = "INTERACTTARGET";
local targetButton = CreateFrame("Button", "BobberMarkerTargetPlayer", UIParent, "SecureActionButtonTemplate");
targetButton:RegisterForClicks("AnyDown", "AnyUp");
targetButton:SetAttribute("type", "target");
targetButton:SetAttribute("unit", "softfriend");
local keysPending = false;
local keyProblems = {};

local function Problem(text)
	keyProblems[#keyProblems + 1] = text;
end

local function FishingName()
	local name;
	if C_Spell and C_Spell.GetSpellName then
		name = C_Spell.GetSpellName(FISHING);
	elseif GetSpellInfo then
		name = GetSpellInfo(FISHING);
	end
	return name or "Fishing";
end

-- Makes sure the account-wide Cast macro casts Fishing. False when it cannot.
local function EnsureCastMacro()
	local body = "#showtooltip\n/cast " .. FishingName();
	local index = GetMacroIndexByName(CAST_MACRO);
	if index and index > 0 then
		local _, _, current = GetMacroInfo(index);
		if current ~= body then
			EditMacro(index, CAST_MACRO, nil, body);
		end
		return true;
	end
	if CreateMacro(CAST_MACRO, MACRO_ICON, body, false) then
		return true;
	end
	Problem("could not create the " .. CAST_MACRO .. " macro; the macro list may be full.");
	return false;
end

local function SetUpKeys()
	local keys = BobberMarkerKeys;
	if type(keys) ~= "table" then
		return;
	end
	if InCombatLockdown() then
		keysPending = true; -- PLAYER_REGEN_ENABLED tries again
		return;
	end
	keysPending = false;
	keyProblems = {};
	local changed = false;
	local cast, interact = keys.cast, keys.interact;
	if cast and cast ~= "" and EnsureCastMacro() and GetBindingAction(cast) ~= "MACRO " .. CAST_MACRO then
		if SetBindingMacro(cast, CAST_MACRO) then
			changed = true;
		else
			Problem("could not bind " .. cast .. "; bind it to the " .. CAST_MACRO .. " macro in Key Bindings.");
		end
	end
	if interact and interact ~= "" and GetBindingAction(interact) ~= INTERACT then
		if SetBinding(interact, INTERACT) then
			changed = true;
		else
			Problem("could not bind " .. interact .. "; bind Interact With Target to it in Key Bindings.");
		end
	end
	ClearOverrideBindings(targetButton);
	local targetPlayer = keys.targetPlayer;
	local softFriend = GetCVarValue("SoftTargetFriend");
	if targetPlayer and targetPlayer ~= "" then
		if GetBindingAction(targetPlayer) == "" then
			SetOverrideBindingClick(targetButton, false, targetPlayer, targetButton:GetName(), "LeftButton");
			if softFriend and softFriend ~= "3" then
				db.softFriendBefore = db.softFriendBefore or softFriend;
				SetCVarValue("SoftTargetFriend", "3");
			end
			if GetCVarValue("SoftTargetFriend") ~= "3" then
				Problem("friendly soft targeting could not be enabled.");
			end
		else
			Problem("the passing-player key " .. targetPlayer .. " is already bound; choose an unused key.");
		end
	elseif db.softFriendBefore then
		if softFriend == "3" then
			SetCVarValue("SoftTargetFriend", db.softFriendBefore);
		end
		db.softFriendBefore = nil;
	end
	if changed then
        SaveBindings(GetCurrentBindingSet());
	end
end

-- A failure here must not stop the marker, so it is kept instead of raised.
local function TrySetUpKeys()
	local ok, err = pcall(SetUpKeys);
	if not ok then
		Problem("could not set up the keys: " .. tostring(err));
	end
end

---------------------------------------------------------------------------
-- Events and the slash command
---------------------------------------------------------------------------

local handlers = {};

-- Called for this addon only; see the event frame below.
function handlers.ADDON_LOADED()
	BobberMarkerDB = BobberMarkerDB or {};
	db = BobberMarkerDB;
	db.y = db.y or DEFAULT_Y;
end

function handlers.PLAYER_ENTERING_WORLD()
	LayoutWorldSignal();
	TrySetUpKeys();
	if IsChanneling() then
		StartFishing();
	else
		StopFishing();
	end
end

function handlers.PLAYER_REGEN_ENABLED()
	if keysPending then
		TrySetUpKeys();
	end
end

function handlers.UNIT_SPELLCAST_CHANNEL_START(unit)
	if unit == "player" then
		StartFishing();
	end
end

function handlers.UNIT_SPELLCAST_CHANNEL_STOP(unit)
	if unit == "player" then
		StopFishing();
	end
end

function handlers.NAME_PLATE_UNIT_REMOVED(unit)
	local ok, plate = pcall(C_NamePlate.GetNamePlateForUnit, unit);
	if ok and plate and plate == anchor then
		Release();
	end
end

handlers.NAME_PLATE_UNIT_ADDED = Evaluate;
handlers.PLAYER_SOFT_INTERACT_CHANGED = Evaluate;
handlers.PLAYER_SOFT_FRIEND_CHANGED = UpdatePlayerSignal;
handlers.PLAYER_TARGET_CHANGED = UpdatePlayerSignal;
handlers.PLAYER_REGEN_DISABLED = function() playerSignal:Hide(); end;
handlers.DISPLAY_SIZE_CHANGED = LayoutWorldSignal;
handlers.UI_SCALE_CHANGED = LayoutWorldSignal;

local addonName = ...;
local events = CreateFrame("Frame");
events:SetScript("OnEvent", function(_, event, arg1)
	if event == "ADDON_LOADED" then
		if arg1 == addonName then
			handlers.ADDON_LOADED();
		end
		return;
	end
	if db then
		handlers[event](arg1);
	end
end);
for event in pairs(handlers) do
	pcall(events.RegisterEvent, events, event);
end

SLASH_BOBBERMARKER1 = "/bobbermarker";
SlashCmdList.BOBBERMARKER = function(input)
	local value = tonumber((input or ""):match("^%s*y%s+(%-?%d+)%s*$"));
	if value then
		db.y = value;
		if anchor then
			local plate = anchor;
			Release();
			Attach(plate);
		end
	end
	print(("|cffff00ffBobber Marker:|r the base vertical offset from the nameplate's centre is %d pixels. Move it with /bobbermarker y <pixels>, negative is down (default %d)."):format(db.y, DEFAULT_Y));
	for _, problem in ipairs(keyProblems) do
		print("|cffff00ffBobber Marker:|r " .. problem);
	end
end
