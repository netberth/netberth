// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

type Notifier struct {
	OnCreate func(resource, id string)
	OnUpdate func(resource, id string)
	OnDelete func(resource, id string)
}

func noopNotifier() *Notifier {
	return &Notifier{
		OnCreate: func(resource, id string) {},
		OnUpdate: func(resource, id string) {},
		OnDelete: func(resource, id string) {},
	}
}
