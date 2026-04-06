/**
 * 亚马逊站点工具函数
 * 根据国家代码（countryCode）获取域名、货币符号等
 * 覆盖所有 Amazon marketplace 表中的 21 个官方站点
 */

// 各站点对应的 Amazon 购物域名（与 001_initial_schema.sql marketplace 表一一对应）
export const AMAZON_DOMAIN_MAP: Record<string, string> = {
  US: 'www.amazon.com',
  CA: 'www.amazon.ca',
  MX: 'www.amazon.com.mx',
  BR: 'www.amazon.com.br',
  GB: 'www.amazon.co.uk',
  DE: 'www.amazon.de',
  FR: 'www.amazon.fr',
  IT: 'www.amazon.it',
  ES: 'www.amazon.es',
  NL: 'www.amazon.nl',
  SE: 'www.amazon.se',
  PL: 'www.amazon.pl',
  TR: 'www.amazon.com.tr',
  AE: 'www.amazon.ae',
  SA: 'www.amazon.sa',
  EG: 'www.amazon.eg',
  IN: 'www.amazon.in',
  BE: 'www.amazon.com.be',
  JP: 'www.amazon.co.jp',
  AU: 'www.amazon.com.au',
  SG: 'www.amazon.sg',
}

/**
 * 根据国家代码返回对应 Amazon 域名
 * @param countryCode 例如 'US', 'GB', 'DE'
 * @returns 例如 'www.amazon.co.uk'
 */
export function getAmazonDomain(countryCode?: string): string {
  return AMAZON_DOMAIN_MAP[countryCode ?? 'US'] ?? 'www.amazon.com'
}

/**
 * 根据国家代码生成商品链接
 */
export function getAsinUrl(asin: string, countryCode?: string): string {
  return `https://${getAmazonDomain(countryCode)}/dp/${asin}`
}

// 货币代码对应的符号（与 003_pricing_replenishment.sql currency_rates 种子数据对应）
export const CURRENCY_SYMBOL_MAP: Record<string, string> = {
  USD: '$',    GBP: '£',    EUR: '€',    JPY: '¥',
  CAD: 'C$',   AUD: 'A$',   INR: '₹',   CNY: '¥',
  MXN: 'MX$',  BRL: 'R$',   TRY: '₺',   SEK: 'kr',
  PLN: 'zł',   AED: 'د.إ', SAR: '﷼',   SGD: 'S$',
  EGP: 'E£',
}

/**
 * 根据货币代码获取货币符号，找不到时回退为"代码 + 空格"
 */
export function getCurrencySymbol(currencyCode?: string): string {
  return CURRENCY_SYMBOL_MAP[currencyCode ?? 'USD'] ?? `${currencyCode ?? 'USD'} `
}

/**
 * 格式化金额显示，带货币符号
 * @example formatAmount(1234.5, 'GBP') => '£1,234.50'
 */
export function formatAmount(value: number, currencyCode?: string, decimals = 2): string {
  const sym = getCurrencySymbol(currencyCode)
  return `${sym}${value.toLocaleString('en-US', { minimumFractionDigits: decimals, maximumFractionDigits: decimals })}`
}
