/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

export function getUserPreferredName(user, fallback = '-') {
  if (!user) return fallback;
  const values = [
    user.contact_name,
    user.contact_qq,
    user.contact_email,
    user.contact_other,
    user.display_name,
    user.user_display_name,
    user.username,
    user.user_username,
  ];
  for (const value of values) {
    const text = String(value || '').trim();
    if (text) return text;
  }
  const id = user.id || user.user_id;
  return id ? `#${id}` : fallback;
}

export function getUserContactItems(user, t = (value) => value) {
  if (!user) return [];
  return [
    { key: 'contact_name', label: t('联系昵称'), value: user.contact_name },
    { key: 'contact_qq', label: 'QQ', value: user.contact_qq },
    {
      key: 'contact_email',
      label: t('联系邮箱'),
      value: user.contact_email,
    },
    {
      key: 'contact_other',
      label: t('其他联系方式'),
      value: user.contact_other,
    },
  ].filter((item) => String(item.value || '').trim());
}
