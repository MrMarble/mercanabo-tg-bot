// Copyright (c) 2020 Sergio Conde skgsergio@gmail.com
//
// This program is free software: you can redistribute it and/or modify it under
// the terms of the GNU General Public License as published by the Free Software
// Foundation, version 3.
//
// This program is distributed in the hope that it will be useful, but WITHOUT ANY
// WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A
// PARTICULAR PURPOSE. See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along with
// this program. If not, see <https://www.gnu.org/licenses/>.
//
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"errors"
	"time"

	tb "gopkg.in/tucnak/telebot.v2"

	"github.com/jinzhu/gorm"

	"github.com/rs/zerolog/log"
)

var (
	// ErrDateParse is returned when an user input date failed to be parsed
	ErrDateParse = errors.New("date parse failed")
)

// GetGroup returns the group updating the its data if changed, if doesnt exist just creates and returns it
func (d *Database) GetGroup(c *tb.Chat) (*Group, bool, error) {
	group := &Group{}
	new := false

	err := d.DB.Where(&Group{ID: c.ID}).First(&group).Error

	if err != nil && !gorm.IsRecordNotFoundError(err) {
		log.Error().Str("module", "database").Err(err).Msg("error getting group")
		return nil, new, err
	}

	if d.DB.NewRecord(group) {
		new = true

		group.ID = c.ID
		group.Title = c.Title
		group.TZ = defaultTZ

		d.DB.Create(&group)
	} else if group.Title != c.Title {
		group.Title = c.Title

		d.DB.Save(&group)
	}

	return group, new, nil
}

// GetUser returns the user updating the its data if changed, if doesnt exist just creates and returns it
func (d *Database) GetUser(u *tb.User) (*User, error) {
	user := &User{}

	err := d.DB.Where(&User{ID: int64(u.ID)}).First(&user).Error

	if err != nil && !gorm.IsRecordNotFoundError(err) {
		log.Error().Str("module", "database").Err(err).Msg("error getting user")
		return nil, err
	}

	if d.DB.NewRecord(user) {
		user.ID = int64(u.ID)
		user.FirstName = u.FirstName
		user.LastName = u.LastName
		user.Username = u.Username

		d.DB.Create(&user)
	} else {
		changed := false
		if user.FirstName != u.FirstName {
			user.FirstName = u.FirstName
			changed = true
		}

		if user.LastName != u.LastName {
			user.LastName = u.LastName
			changed = true
		}

		if user.Username != u.Username {
			user.Username = u.Username
			changed = true
		}

		if changed {
			d.DB.Save(&user)
		}
	}

	return user, nil
}

// GetUserAndGroup returns the
func (d *Database) GetUserAndGroup(u *tb.User, c *tb.Chat) (*User, *Group, error) {
	// Get user
	user, err := d.GetUser(u)
	if err != nil {
		return nil, nil, err
	}

	// Get group
	group, _, err := d.GetGroup(c)
	if err != nil {
		return user, nil, err
	}

	return user, group, nil
}

// getThisWeekOwned returns owned turnips by the user this week
func (d *Database) getThisWeekOwned(u *User, g *Group) (*Owned, error) {
	// Get now config with group timezone
	nowCfg, err := g.NowConfig()
	if err != nil {
		return nil, err
	}

	bowDate := nowCfg.With(time.Now().In(nowCfg.TimeLocation)).BeginningOfWeek()
	eowDate := nowCfg.With(bowDate).EndOfWeek()

	// Get this week owned
	owned := &Owned{}

	err = d.DB.Where("user_id = ? AND group_id = ? AND date >= ? AND date <= ?",
		u.ID,
		g.ID,
		bowDate,
		eowDate,
	).First(&owned).Error

	if err != nil && !gorm.IsRecordNotFoundError(err) {
		log.Error().Str("module", "database").Err(err).Msg("error getting owned")
		return nil, err
	}

	return owned, nil
}

// GetThisWeekOwned returns owned turnips by the user this week
func (d *Database) GetThisWeekOwned(u *tb.User, c *tb.Chat) (*Owned, error) {
	user, group, err := d.GetUserAndGroup(u, c)
	if err != nil {
		return nil, err
	}

	return d.getThisWeekOwned(user, group)
}

// SaveThisWeekOwned sets owned turnips by the user this week
func (d *Database) SaveThisWeekOwned(u *tb.User, c *tb.Chat, units uint32, bells uint32) (bool, uint32, uint32, error) {
	// Get user and group
	user, group, err := d.GetUserAndGroup(u, c)
	if err != nil {
		return false, 0, 0, err
	}

	// Get now config with group timezone
	nowCfg, err := group.NowConfig()
	if err != nil {
		return false, 0, 0, err
	}

	// Get current week owneds if exists
	owned, err := db.getThisWeekOwned(user, group)
	if err != nil {
		return false, 0, 0, err
	}

	new := db.DB.NewRecord(owned)
	oldUnits := owned.Units
	oldBells := owned.Bells

	owned.UserID = user.ID
	owned.GroupID = group.ID
	owned.Units = units
	owned.Bells = bells
	owned.Date = nowCfg.With(time.Now().In(nowCfg.TimeLocation)).BeginningOfWeek()

	if new {
		err = d.DB.Create(&owned).Error
	} else {
		err = d.DB.Save(&owned).Error
	}

	if err != nil {
		log.Error().Str("module", "database").Err(err).Bool("new", new).Msg("error saving owned")
	}

	return new, oldUnits, oldBells, err
}

// getUserSellPrice gets sell price at Nook's Cranny of an User in a Group at a given time
func (d *Database) getUserSellPrice(u *User, g *Group, t time.Time) (*Price, error) {
	// Get now config with group timezone
	nowCfg, err := g.NowConfig()
	if err != nil {
		return nil, err
	}

	// Get the correct date
	reqDate := t.In(nowCfg.TimeLocation)

	amDate := nowCfg.With(reqDate).BeginningOfDay()
	pmDate := amDate.Add(time.Hour * 12)

	if reqDate.Before(pmDate) {
		reqDate = amDate
	} else {
		reqDate = pmDate
	}

	// Get price
	price := &Price{}

	err = d.DB.Where("user_id = ? AND group_id = ? AND date = ?",
		u.ID,
		g.ID,
		reqDate,
	).First(&price).Error

	if err != nil && !gorm.IsRecordNotFoundError(err) {
		log.Error().Str("module", "database").Err(err).Msg("error getting price")
		return nil, err
	}

	return price, nil
}

// GetSellPrice gets sell price at Nook's Cranny at a given time
func (d *Database) GetSellPrice(u *tb.User, c *tb.Chat, t *time.Time) (*Price, error) {
	return nil, nil
}

// GetCurrentSellPrice gets current sell price at Nook's Cranny
func (d *Database) GetCurrentSellPrice(u *tb.User, c *tb.Chat) (*Price, error) {
	return nil, nil
}

// saveSellPrice sets sell price at Nook's Cranny at a given time
func (d *Database) saveSellPrice(u *User, g *Group, bells uint32, t time.Time) (bool, uint32, string, error) {
	// Save price
	price, err := d.getUserSellPrice(u, g, t)
	if err != nil {
		return false, 0, "", err
	}

	new := db.DB.NewRecord(price)
	oldBells := price.Bells

	price.UserID = u.ID
	price.GroupID = g.ID
	price.Bells = bells
	price.Date = t

	if new {
		err = d.DB.Create(&price).Error
	} else {
		err = d.DB.Save(&price).Error
	}

	if err != nil {
		log.Error().Str("module", "database").Err(err).Bool("new", new).Msg("error saving price")
	}

	return new, oldBells, t.Format(timeFormatAMPM), nil
}

// SaveSellPrice sets sell price at Nook's Cranny at a given time
func (d *Database) SaveSellPrice(u *tb.User, c *tb.Chat, bells uint32, dateStr string) (bool, uint32, string, error) {
	// Get user and group
	user, group, err := d.GetUserAndGroup(u, c)
	if err != nil {
		return false, 0, "", err
	}

	// Get now config with group timezone
	nowCfg, err := group.NowConfig()
	if err != nil {
		return false, 0, "", err
	}

	// Parse date
	date, err := nowCfg.Parse(dateStr)
	if err != nil {
		return false, 0, "", ErrDateParse
	}

	// Save price
	return d.saveSellPrice(user, group, bells, date)
}

// SaveCurrentSellPrice sets current sell price at Nook's Cranny
func (d *Database) SaveCurrentSellPrice(u *tb.User, c *tb.Chat, bells uint32) (bool, uint32, string, error) {
	// Get user and group
	user, group, err := d.GetUserAndGroup(u, c)
	if err != nil {
		return false, 0, "", err
	}

	// Get now config with group timezone
	nowCfg, err := group.NowConfig()
	if err != nil {
		return false, 0, "", err
	}

	// Get current date and set it to 00:00:00 (AM) or 12:00:00 (PM)
	currentDate := time.Now().In(nowCfg.TimeLocation)

	amDate := nowCfg.With(currentDate).BeginningOfDay()
	pmDate := amDate.Add(time.Hour * 12)

	if currentDate.Before(pmDate) {
		currentDate = amDate
	} else {
		currentDate = pmDate
	}

	// Save price
	return d.saveSellPrice(user, group, bells, currentDate)
}