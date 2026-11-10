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
-- The bobber looks bigger and further below its nameplate as the camera
-- comes closer, so BOX and the offset hold at REF_ZOOM yards of camera
-- distance and scale with (REF_ZOOM + CAST_DISTANCE) / (zoom + CAST_DISTANCE).
local REF_ZOOM = 15;      -- the game's full zoom-out without the extended camera
local CAST_DISTANCE = 20; -- roughly how far in front of you the bobber lands
local MIN_SCALE, MAX_SCALE = 0.5, 4; -- keeps the box within Go Fish's 30 to 400 pixels
local COLOR = { 1, 0, 1 }; -- opaque magenta: exact over any background, and rare in the world

local POLL = 0.1; -- seconds between checks while fishing

-- Soft targeting must be on for the bobber to become the soft target. These
-- are checked each time you start fishing: the game keeps some per character,
-- and a copied WTF folder can carry another account's saved variables.
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
local scale = 1;   -- the zoom scale the marker was attached with
local attachedPx = 1; -- PixelSize(marker) when it was attached

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


-- Tells Go Fish how each cast ended, so it can count catches and tell a reel
-- on a false bite ("No fish are hooked") from a fish that got away. Yellow,
-- cyan and yellow blocks under the in-world blocks, then a block whose red is
-- a counter that changes with every outcome and whose green is the outcome.
local outcomeSignal = CreateFrame("Frame");
outcomeSignal:SetFrameStrata("TOOLTIP");
outcomeSignal:Hide();
local outcomeBlocks = {};
for i, rgb in ipairs({ { 1, 1, 0 }, { 0, 1, 1 }, { 1, 1, 0 }, { 0, 0, 0 } }) do
    local block = outcomeSignal:CreateTexture(nil, "OVERLAY");
    block:SetColorTexture(rgb[1], rgb[2], rgb[3], 1);
    outcomeBlocks[i] = block;
end
local outcomeSequence = 0;
-- The green of the last block for each outcome; Go Fish reads the same values.
local OUTCOME_CAUGHT, OUTCOME_GOT_AWAY, OUTCOME_NOT_HOOKED = 64, 128, 192;

local function LayoutOutcomeSignal()
    local px = PixelSize(outcomeSignal);
    outcomeSignal:ClearAllPoints();
    outcomeSignal:SetPoint("TOPLEFT", WorldFrame, "TOPLEFT", 8 * px, -24 * px);
    outcomeSignal:SetSize(24 * px, 6 * px);
    for i, block in ipairs(outcomeBlocks) do
        block:SetPoint("TOPLEFT", outcomeSignal, "TOPLEFT", (i - 1) * 6 * px, 0);
        block:SetSize(6 * px, 6 * px);
    end
end

local function ShowOutcome(code)
    outcomeSequence = outcomeSequence % 255 + 1;
    outcomeBlocks[4]:SetColorTexture(outcomeSequence / 255, code / 255, 77 / 255, 1);
    LayoutOutcomeSignal();
    outcomeSignal:Show();
end

-- The game's messages for a fish that got away and for a reel with nothing on
-- the hook: the client's own wording where it has a name for it, and the
-- English text, lower case without the closing punctuation.
local function Normal(text)
    return (text:lower():gsub("[%s%.!]+$", ""));
end
local function Messages(english, ...)
    local list = { Normal(english) };
    for _, name in ipairs({ ... }) do
        local text = _G[name];
        if type(text) == "string" and text ~= "" then
            table.insert(list, Normal(text));
        end
    end
    return list;
end
local GOT_AWAY = Messages("Your fish got away!", "ERR_FISH_ESCAPED", "SPELL_FAILED_FISH_ESCAPED");
local NOT_HOOKED = Messages("No fish are hooked.", "ERR_FISH_NOT_HOOKED", "SPELL_FAILED_NO_FISH");

local function Matches(text, list)
    for _, message in ipairs(list) do
        if text:find(message, 1, true) then
            return true;
        end
    end
    return false;
end

-- Reads every message the game shows in its yellow error text, whichever
-- event brought it, and reports the two fishing ones. The same outcome twice
-- within a second is one message shown twice.
local lastOutcome, lastOutcomeAt = 0, 0;
local function OnErrorText(_, text)
    if type(text) ~= "string" then
        return;
    end
    text = Normal(text);
    local code = Matches(text, GOT_AWAY) and OUTCOME_GOT_AWAY or Matches(text, NOT_HOOKED) and OUTCOME_NOT_HOOKED;
    if not code or code == lastOutcome and GetTime() - lastOutcomeAt < 1 then
        return;
    end
    lastOutcome, lastOutcomeAt = code, GetTime();
    ShowOutcome(code);
end
if UIErrorsFrame then
    hooksecurefunc(UIErrorsFrame, "AddMessage", OnErrorText);
end
-- How much bigger the bobber looks than at REF_ZOOM; 1 on a client without GetCameraZoom.
local function ZoomScale()
	local ok, zoom = pcall(GetCameraZoom);
	if not ok or not Readable(zoom) or type(zoom) ~= "number" then
		return 1;
	end
	local s = (REF_ZOOM + CAST_DISTANCE) / (zoom + CAST_DISTANCE);
	return math.min(math.max(s, MIN_SCALE), MAX_SCALE);
end

local function Layout()
	local px = PixelSize(marker);
	marker:SetSize(BOX * scale * px, BOX * scale * px);
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
	if marker.SetIgnoreParentScale then
		marker:SetIgnoreParentScale(true); -- the plate grows as it settles, the box must not
	end
	local ok, level = pcall(plate.GetFrameLevel, plate);
	if ok and Readable(level) then
		marker:SetFrameLevel(level + 10); -- above the nameplate's own parts
	end
	scale = ZoomScale();
	Layout();
	marker:ClearAllPoints();
	local px = PixelSize(marker);
	marker:SetPoint("CENTER", plate, "CENTER", offsetX * px, (db.y * scale + offsetY) * px);
	marker:Show();
	anchor, attachedPx = plate, px;
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
	-- Small steps are ignored, so the watched box stays put through camera jitter.
	-- The pixel check covers a client without SetIgnoreParentScale.
	if plate ~= anchor or math.abs(ZoomScale() - scale) > 0.05 * scale
		or math.abs(PixelSize(marker) - attachedPx) > 0.05 * attachedPx then
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
	if not InCombatLockdown() then
		for name, value in pairs(NEEDED) do
			SetCVarValue(name, value);
		end
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
-- Interact key, and the Logout key when it takes breaks. Its installer writes
-- them into BobberMarkerKeys (Keys.lua, loaded before this file), and this puts
-- them to work in the game so nobody has to: a macro that casts Fishing on the
-- Cast key, the game's own Interact action on the Interact key, and a macro
-- that logs out on the Logout key. Macros and bindings cannot change in combat, so
-- this waits for it to end. It changes only what differs and never writes to the
-- chat; what went wrong is kept in keyProblems for /bobbermarker to show.

local CAST_MACRO = "Fishing";
local LOGOUT_MACRO = "Logout";
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

-- Makes sure the account-wide macro name holds body. False when it cannot.
local function EnsureMacro(name, body)
	local index = GetMacroIndexByName(name);
	if index and index > 0 then
		local _, _, current = GetMacroInfo(index);
		if current ~= body then
			EditMacro(index, name, nil, body);
		end
		return true;
	end
	if CreateMacro(name, MACRO_ICON, body, false) then
		return true;
	end
	Problem("could not create the " .. name .. " macro; the macro list may be full.");
	return false;
end

-- Binds key to the macro name holding body. True when the binding changed.
local function BindMacro(key, name, body)
	if not key or key == "" or not EnsureMacro(name, body) or GetBindingAction(key) == "MACRO " .. name then
		return false;
	end
	if SetBindingMacro(key, name) then
		return true;
	end
	Problem("could not bind " .. key .. "; bind it to the " .. name .. " macro in Key Bindings.");
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
	local interact = keys.interact;
	changed = BindMacro(keys.cast, CAST_MACRO, "#showtooltip\n/cast " .. FishingName()) or changed;
	changed = BindMacro(keys.logout, LOGOUT_MACRO, "/logout") or changed;
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

-- The error event as well, for a client whose error frame shows messages some
-- other way; OnErrorText takes the same message only once. arg1 is the
-- message type and arg2 the text on current clients; older ones pass only the text.
function handlers.UI_ERROR_MESSAGE(arg1, arg2)
	OnErrorText(nil, type(arg2) == "string" and arg2 or arg1);
end

-- A fast-loot addon takes everything on LOOT_READY, and LOOT_OPENED may then
-- never come, so both report the catch; the second within a second is the same loot.
local lastCatchAt = 0;
local function OnLoot()
	if IsFishingLoot and IsFishingLoot() and GetTime() - lastCatchAt >= 1 then
		lastCatchAt = GetTime();
		ShowOutcome(OUTCOME_CAUGHT);
	end
end
handlers.LOOT_READY = OnLoot;
handlers.LOOT_OPENED = OnLoot;

handlers.NAME_PLATE_UNIT_ADDED = Evaluate;
handlers.PLAYER_SOFT_INTERACT_CHANGED = Evaluate;
handlers.PLAYER_SOFT_FRIEND_CHANGED = UpdatePlayerSignal;
handlers.PLAYER_TARGET_CHANGED = UpdatePlayerSignal;
handlers.PLAYER_REGEN_DISABLED = function() playerSignal:Hide(); end;
handlers.DISPLAY_SIZE_CHANGED = function() LayoutWorldSignal(); LayoutOutcomeSignal(); end;
handlers.UI_SCALE_CHANGED = handlers.DISPLAY_SIZE_CHANGED;

local addonName = ...;
local events = CreateFrame("Frame");
events:SetScript("OnEvent", function(_, event, arg1, arg2)
	if event == "ADDON_LOADED" then
		if arg1 == addonName then
			handlers.ADDON_LOADED();
		end
		return;
	end
	if db then
		handlers[event](arg1, arg2);
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
	print(("|cffff00ffBobber Marker:|r the base vertical offset from the nameplate's centre is %d pixels at full zoom-out. Move it with /bobbermarker y <pixels>, negative is down (default %d). At this zoom the box and offset are x%.2f."):format(db.y, DEFAULT_Y, ZoomScale()));
	for _, problem in ipairs(keyProblems) do
		print("|cffff00ffBobber Marker:|r " .. problem);
	end
end
