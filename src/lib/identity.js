/**
 * @param {string} name
 * @returns {string}
 */
export function monogram(name) {
  const clean = name.trim();
  if (!clean) return "";
  return clean[0].toUpperCase();
}
