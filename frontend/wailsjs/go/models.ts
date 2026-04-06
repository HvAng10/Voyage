export namespace main {
	
	export class AccountInfo {
	    id: number;
	    name: string;
	    sellerId: string;
	    region: string;
	    isActive: boolean;
	    marketplaces: string[];
	
	    static createFrom(source: any = {}) {
	        return new AccountInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.sellerId = source["sellerId"];
	        this.region = source["region"];
	        this.isActive = source["isActive"];
	        this.marketplaces = source["marketplaces"];
	    }
	}
	export class AdCampaignSummary {
	    campaignId: string;
	    name: string;
	    state: string;
	    dailyBudget: number;
	    totalCost: number;
	    totalSales: number;
	    totalClicks: number;
	    impressions: number;
	    acos: number;
	    roas: number;
	    ctr: number;
	
	    static createFrom(source: any = {}) {
	        return new AdCampaignSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.campaignId = source["campaignId"];
	        this.name = source["name"];
	        this.state = source["state"];
	        this.dailyBudget = source["dailyBudget"];
	        this.totalCost = source["totalCost"];
	        this.totalSales = source["totalSales"];
	        this.totalClicks = source["totalClicks"];
	        this.impressions = source["impressions"];
	        this.acos = source["acos"];
	        this.roas = source["roas"];
	        this.ctr = source["ctr"];
	    }
	}
	export class MarketplaceInfo {
	    marketplaceId: string;
	    countryCode: string;
	    name: string;
	    currencyCode: string;
	    region: string;
	    timezone: string;
	
	    static createFrom(source: any = {}) {
	        return new MarketplaceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.marketplaceId = source["marketplaceId"];
	        this.countryCode = source["countryCode"];
	        this.name = source["name"];
	        this.currencyCode = source["currencyCode"];
	        this.region = source["region"];
	        this.timezone = source["timezone"];
	    }
	}

}

export namespace services {
	
	export class AccountKPISummary {
	    accountId: number;
	    accountName: string;
	    marketplaceId: string;
	    marketplaceName: string;
	    originalCurrency: string;
	    sales: number;
	    salesCny: number;
	    orders: number;
	    adSpend: number;
	    adSpendCny: number;
	    fees: number;
	    feesCny: number;
	    cogs: number;
	    cogsCny: number;
	    netProfit: number;
	    netProfitCny: number;
	    profitMargin: number;
	    profitSharePct: number;
	
	    static createFrom(source: any = {}) {
	        return new AccountKPISummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.accountId = source["accountId"];
	        this.accountName = source["accountName"];
	        this.marketplaceId = source["marketplaceId"];
	        this.marketplaceName = source["marketplaceName"];
	        this.originalCurrency = source["originalCurrency"];
	        this.sales = source["sales"];
	        this.salesCny = source["salesCny"];
	        this.orders = source["orders"];
	        this.adSpend = source["adSpend"];
	        this.adSpendCny = source["adSpendCny"];
	        this.fees = source["fees"];
	        this.feesCny = source["feesCny"];
	        this.cogs = source["cogs"];
	        this.cogsCny = source["cogsCny"];
	        this.netProfit = source["netProfit"];
	        this.netProfitCny = source["netProfitCny"];
	        this.profitMargin = source["profitMargin"];
	        this.profitSharePct = source["profitSharePct"];
	    }
	}
	export class AdKeywordRow {
	    campaignName: string;
	    adGroupName: string;
	    keywordText: string;
	    matchType: string;
	    state: string;
	    impressions: number;
	    clicks: number;
	    cost: number;
	    sales: number;
	    orders: number;
	    acos: number;
	    ctr: number;
	    cpc: number;
	
	    static createFrom(source: any = {}) {
	        return new AdKeywordRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.campaignName = source["campaignName"];
	        this.adGroupName = source["adGroupName"];
	        this.keywordText = source["keywordText"];
	        this.matchType = source["matchType"];
	        this.state = source["state"];
	        this.impressions = source["impressions"];
	        this.clicks = source["clicks"];
	        this.cost = source["cost"];
	        this.sales = source["sales"];
	        this.orders = source["orders"];
	        this.acos = source["acos"];
	        this.ctr = source["ctr"];
	        this.cpc = source["cpc"];
	    }
	}
	export class AdOverview {
	    totalCampaigns: number;
	    totalSpend: number;
	    totalSales: number;
	    avgAcos: number;
	    avgRoas: number;
	
	    static createFrom(source: any = {}) {
	        return new AdOverview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalCampaigns = source["totalCampaigns"];
	        this.totalSpend = source["totalSpend"];
	        this.totalSales = source["totalSales"];
	        this.avgAcos = source["avgAcos"];
	        this.avgRoas = source["avgRoas"];
	    }
	}
	export class AdTargetRow {
	    campaignName: string;
	    targetType: string;
	    targetValue: string;
	    state: string;
	    impressions: number;
	    clicks: number;
	    cost: number;
	    sales: number;
	    acos: number;
	
	    static createFrom(source: any = {}) {
	        return new AdTargetRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.campaignName = source["campaignName"];
	        this.targetType = source["targetType"];
	        this.targetValue = source["targetValue"];
	        this.state = source["state"];
	        this.impressions = source["impressions"];
	        this.clicks = source["clicks"];
	        this.cost = source["cost"];
	        this.sales = source["sales"];
	        this.acos = source["acos"];
	    }
	}
	export class Alert {
	    id: number;
	    alertType: string;
	    severity: string;
	    title: string;
	    message: string;
	    relatedEntityType: string;
	    relatedEntityId: string;
	    isRead: boolean;
	    isDismissed: boolean;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Alert(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.alertType = source["alertType"];
	        this.severity = source["severity"];
	        this.title = source["title"];
	        this.message = source["message"];
	        this.relatedEntityType = source["relatedEntityType"];
	        this.relatedEntityId = source["relatedEntityId"];
	        this.isRead = source["isRead"];
	        this.isDismissed = source["isDismissed"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class AsinDailyPoint {
	    date: string;
	    sales: number;
	    units: number;
	    sessions: number;
	
	    static createFrom(source: any = {}) {
	        return new AsinDailyPoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date = source["date"];
	        this.sales = source["sales"];
	        this.units = source["units"];
	        this.sessions = source["sessions"];
	    }
	}
	export class AsinFeeInfo {
	    totalRevenue: number;
	    totalFbaFee: number;
	    totalRefFee: number;
	    totalAdSpend: number;
	    actualFeeRate: number;
	    dataNote: string;
	
	    static createFrom(source: any = {}) {
	        return new AsinFeeInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalRevenue = source["totalRevenue"];
	        this.totalFbaFee = source["totalFbaFee"];
	        this.totalRefFee = source["totalRefFee"];
	        this.totalAdSpend = source["totalAdSpend"];
	        this.actualFeeRate = source["actualFeeRate"];
	        this.dataNote = source["dataNote"];
	    }
	}
	export class BidSuggestion {
	    campaignName: string;
	    adGroupName: string;
	    keywordText: string;
	    matchType: string;
	    histImpressions: number;
	    histClicks: number;
	    histCost: number;
	    histSales: number;
	    histOrders: number;
	    histAcos: number;
	    histCtr: number;
	    histCpc: number;
	    histCvr: number;
	    suggestedBid: number;
	    currentBid: number;
	    bidDelta: number;
	    confidence: string;
	    reason: string;
	    targetAcos: number;
	    avgSalePrice: number;
	
	    static createFrom(source: any = {}) {
	        return new BidSuggestion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.campaignName = source["campaignName"];
	        this.adGroupName = source["adGroupName"];
	        this.keywordText = source["keywordText"];
	        this.matchType = source["matchType"];
	        this.histImpressions = source["histImpressions"];
	        this.histClicks = source["histClicks"];
	        this.histCost = source["histCost"];
	        this.histSales = source["histSales"];
	        this.histOrders = source["histOrders"];
	        this.histAcos = source["histAcos"];
	        this.histCtr = source["histCtr"];
	        this.histCpc = source["histCpc"];
	        this.histCvr = source["histCvr"];
	        this.suggestedBid = source["suggestedBid"];
	        this.currentBid = source["currentBid"];
	        this.bidDelta = source["bidDelta"];
	        this.confidence = source["confidence"];
	        this.reason = source["reason"];
	        this.targetAcos = source["targetAcos"];
	        this.avgSalePrice = source["avgSalePrice"];
	    }
	}
	export class CompetitivePriceItem {
	    asin: string;
	    sku: string;
	    title: string;
	    listingPrice: number;
	    buyBoxPrice: number;
	    isBuyBoxWinner: boolean;
	    numberOfOffers: number;
	    snapshotDate: string;
	    prevBuyBoxPrice: number;
	    priceChangePct: number;
	    buyBoxLostDays: number;
	
	    static createFrom(source: any = {}) {
	        return new CompetitivePriceItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.asin = source["asin"];
	        this.sku = source["sku"];
	        this.title = source["title"];
	        this.listingPrice = source["listingPrice"];
	        this.buyBoxPrice = source["buyBoxPrice"];
	        this.isBuyBoxWinner = source["isBuyBoxWinner"];
	        this.numberOfOffers = source["numberOfOffers"];
	        this.snapshotDate = source["snapshotDate"];
	        this.prevBuyBoxPrice = source["prevBuyBoxPrice"];
	        this.priceChangePct = source["priceChangePct"];
	        this.buyBoxLostDays = source["buyBoxLostDays"];
	    }
	}
	export class CostImportResult {
	    imported: number;
	    errors: string[];
	
	    static createFrom(source: any = {}) {
	        return new CostImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.imported = source["imported"];
	        this.errors = source["errors"];
	    }
	}
	export class CrossAccountKPI {
	    totalSalesCny: number;
	    totalOrderCount: number;
	    totalAdSpendCny: number;
	    totalFeesCny: number;
	    totalCogsCny: number;
	    totalNetProfitCny: number;
	    totalProfitMargin: number;
	    accountBreakdown: AccountKPISummary[];
	    baseCurrency: string;
	    rateUpdatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new CrossAccountKPI(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalSalesCny = source["totalSalesCny"];
	        this.totalOrderCount = source["totalOrderCount"];
	        this.totalAdSpendCny = source["totalAdSpendCny"];
	        this.totalFeesCny = source["totalFeesCny"];
	        this.totalCogsCny = source["totalCogsCny"];
	        this.totalNetProfitCny = source["totalNetProfitCny"];
	        this.totalProfitMargin = source["totalProfitMargin"];
	        this.accountBreakdown = this.convertValues(source["accountBreakdown"], AccountKPISummary);
	        this.baseCurrency = source["baseCurrency"];
	        this.rateUpdatedAt = source["rateUpdatedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CurrencyRate {
	    currencyCode: string;
	    rateToCny: number;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new CurrencyRate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.currencyCode = source["currencyCode"];
	        this.rateToCny = source["rateToCny"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class DailyDataPoint {
	    date: string;
	    sales: number;
	    orders: number;
	    units: number;
	    adSpend: number;
	    pageViews: number;
	    sessions: number;
	    conversionRate: number;
	    acos: number;
	
	    static createFrom(source: any = {}) {
	        return new DailyDataPoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date = source["date"];
	        this.sales = source["sales"];
	        this.orders = source["orders"];
	        this.units = source["units"];
	        this.adSpend = source["adSpend"];
	        this.pageViews = source["pageViews"];
	        this.sessions = source["sessions"];
	        this.conversionRate = source["conversionRate"];
	        this.acos = source["acos"];
	    }
	}
	export class DailyProfitCell {
	    date: string;
	    sales: number;
	    adSpend: number;
	    fees: number;
	    cogs: number;
	    netProfit: number;
	    level: string;
	
	    static createFrom(source: any = {}) {
	        return new DailyProfitCell(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date = source["date"];
	        this.sales = source["sales"];
	        this.adSpend = source["adSpend"];
	        this.fees = source["fees"];
	        this.cogs = source["cogs"];
	        this.netProfit = source["netProfit"];
	        this.level = source["level"];
	    }
	}
	export class DashboardKPI {
	    totalSales: number;
	    salesPrev: number;
	    salesTrend: number;
	    totalOrders: number;
	    ordersPrev: number;
	    ordersTrend: number;
	    adSpend: number;
	    adSpendPrev: number;
	    adSpendTrend: number;
	    acos: number;
	    acosPrev: number;
	    acosTrend: number;
	    netProfit: number;
	    netProfitTrend: number;
	    currency: string;
	    dateStart: string;
	    dateEnd: string;
	    dataLatencyDays: number;
	
	    static createFrom(source: any = {}) {
	        return new DashboardKPI(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalSales = source["totalSales"];
	        this.salesPrev = source["salesPrev"];
	        this.salesTrend = source["salesTrend"];
	        this.totalOrders = source["totalOrders"];
	        this.ordersPrev = source["ordersPrev"];
	        this.ordersTrend = source["ordersTrend"];
	        this.adSpend = source["adSpend"];
	        this.adSpendPrev = source["adSpendPrev"];
	        this.adSpendTrend = source["adSpendTrend"];
	        this.acos = source["acos"];
	        this.acosPrev = source["acosPrev"];
	        this.acosTrend = source["acosTrend"];
	        this.netProfit = source["netProfit"];
	        this.netProfitTrend = source["netProfitTrend"];
	        this.currency = source["currency"];
	        this.dateStart = source["dateStart"];
	        this.dateEnd = source["dateEnd"];
	        this.dataLatencyDays = source["dataLatencyDays"];
	    }
	}
	export class FinanceSummary {
	    totalRevenue: number;
	    totalRefunds: number;
	    totalFees: number;
	    totalAdSpend: number;
	    totalCogs: number;
	    grossProfit: number;
	    netProfit: number;
	    profitMargin: number;
	    currency: string;
	    dateStart: string;
	    dateEnd: string;
	
	    static createFrom(source: any = {}) {
	        return new FinanceSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalRevenue = source["totalRevenue"];
	        this.totalRefunds = source["totalRefunds"];
	        this.totalFees = source["totalFees"];
	        this.totalAdSpend = source["totalAdSpend"];
	        this.totalCogs = source["totalCogs"];
	        this.grossProfit = source["grossProfit"];
	        this.netProfit = source["netProfit"];
	        this.profitMargin = source["profitMargin"];
	        this.currency = source["currency"];
	        this.dateStart = source["dateStart"];
	        this.dateEnd = source["dateEnd"];
	    }
	}
	export class InventoryItem {
	    sellerSku: string;
	    asin: string;
	    title: string;
	    fulfillableQty: number;
	    inboundQty: number;
	    unsellableQty: number;
	    reservedQty: number;
	    totalQty: number;
	    dailySalesAvg: number;
	    estDaysLeft: number;
	    alertLevel: string;
	    snapshotDate: string;
	
	    static createFrom(source: any = {}) {
	        return new InventoryItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sellerSku = source["sellerSku"];
	        this.asin = source["asin"];
	        this.title = source["title"];
	        this.fulfillableQty = source["fulfillableQty"];
	        this.inboundQty = source["inboundQty"];
	        this.unsellableQty = source["unsellableQty"];
	        this.reservedQty = source["reservedQty"];
	        this.totalQty = source["totalQty"];
	        this.dailySalesAvg = source["dailySalesAvg"];
	        this.estDaysLeft = source["estDaysLeft"];
	        this.alertLevel = source["alertLevel"];
	        this.snapshotDate = source["snapshotDate"];
	    }
	}
	export class InventoryOverview {
	    totalSku: number;
	    criticalCount: number;
	    warningCount: number;
	    okCount: number;
	    totalFulfillable: number;
	
	    static createFrom(source: any = {}) {
	        return new InventoryOverview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalSku = source["totalSku"];
	        this.criticalCount = source["criticalCount"];
	        this.warningCount = source["warningCount"];
	        this.okCount = source["okCount"];
	        this.totalFulfillable = source["totalFulfillable"];
	    }
	}
	export class PlacementRow {
	    campaignName: string;
	    campaignId: string;
	    placement: string;
	    impressions: number;
	    clicks: number;
	    cost: number;
	    sales7d: number;
	    orders7d: number;
	    acos: number;
	    ctr: number;
	    cpc: number;
	    cvr: number;
	
	    static createFrom(source: any = {}) {
	        return new PlacementRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.campaignName = source["campaignName"];
	        this.campaignId = source["campaignId"];
	        this.placement = source["placement"];
	        this.impressions = source["impressions"];
	        this.clicks = source["clicks"];
	        this.cost = source["cost"];
	        this.sales7d = source["sales7d"];
	        this.orders7d = source["orders7d"];
	        this.acos = source["acos"];
	        this.ctr = source["ctr"];
	        this.cpc = source["cpc"];
	        this.cvr = source["cvr"];
	    }
	}
	export class PriceAlertConfig {
	    accountId: number;
	    priceDropThreshold: number;
	    priceSurgeThreshold: number;
	    buyBoxCriticalHours: number;
	
	    static createFrom(source: any = {}) {
	        return new PriceAlertConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.accountId = source["accountId"];
	        this.priceDropThreshold = source["priceDropThreshold"];
	        this.priceSurgeThreshold = source["priceSurgeThreshold"];
	        this.buyBoxCriticalHours = source["buyBoxCriticalHours"];
	    }
	}
	export class ProfitMarginPoint {
	    date: string;
	    sales: number;
	    netProfit: number;
	    margin: number;
	    adSpend: number;
	    fees: number;
	    cogs: number;
	
	    static createFrom(source: any = {}) {
	        return new ProfitMarginPoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date = source["date"];
	        this.sales = source["sales"];
	        this.netProfit = source["netProfit"];
	        this.margin = source["margin"];
	        this.adSpend = source["adSpend"];
	        this.fees = source["fees"];
	        this.cogs = source["cogs"];
	    }
	}
	export class ReplenishmentAdvice {
	    sku: string;
	    asin: string;
	    title: string;
	    currentStock: number;
	    inboundQty: number;
	    reservedQty: number;
	    dailyAvgSales: number;
	    seasonFactor: number;
	    effectiveDailyAvg: number;
	    daysOfStock: number;
	    leadTimeDays: number;
	    safetyDays: number;
	    targetDays: number;
	    reorderPoint: number;
	    suggestedQty: number;
	    urgency: string;
	    estCost: number;
	    unitCost: number;
	
	    static createFrom(source: any = {}) {
	        return new ReplenishmentAdvice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sku = source["sku"];
	        this.asin = source["asin"];
	        this.title = source["title"];
	        this.currentStock = source["currentStock"];
	        this.inboundQty = source["inboundQty"];
	        this.reservedQty = source["reservedQty"];
	        this.dailyAvgSales = source["dailyAvgSales"];
	        this.seasonFactor = source["seasonFactor"];
	        this.effectiveDailyAvg = source["effectiveDailyAvg"];
	        this.daysOfStock = source["daysOfStock"];
	        this.leadTimeDays = source["leadTimeDays"];
	        this.safetyDays = source["safetyDays"];
	        this.targetDays = source["targetDays"];
	        this.reorderPoint = source["reorderPoint"];
	        this.suggestedQty = source["suggestedQty"];
	        this.urgency = source["urgency"];
	        this.estCost = source["estCost"];
	        this.unitCost = source["unitCost"];
	    }
	}
	export class ReturnDetail {
	    returnDate: string;
	    sku: string;
	    asin: string;
	    quantity: number;
	    reason: string;
	    detailedDisposition: string;
	    status: string;
	    fulfillmentCenter: string;
	
	    static createFrom(source: any = {}) {
	        return new ReturnDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.returnDate = source["returnDate"];
	        this.sku = source["sku"];
	        this.asin = source["asin"];
	        this.quantity = source["quantity"];
	        this.reason = source["reason"];
	        this.detailedDisposition = source["detailedDisposition"];
	        this.status = source["status"];
	        this.fulfillmentCenter = source["fulfillmentCenter"];
	    }
	}
	export class ReturnRateByASIN {
	    asin: string;
	    sku: string;
	    title: string;
	    totalReturns: number;
	    totalSold: number;
	    returnRate: number;
	    topReason: string;
	    dataLatencyNote: string;
	
	    static createFrom(source: any = {}) {
	        return new ReturnRateByASIN(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.asin = source["asin"];
	        this.sku = source["sku"];
	        this.title = source["title"];
	        this.totalReturns = source["totalReturns"];
	        this.totalSold = source["totalSold"];
	        this.returnRate = source["returnRate"];
	        this.topReason = source["topReason"];
	        this.dataLatencyNote = source["dataLatencyNote"];
	    }
	}
	export class ReturnReasonStat {
	    reason: string;
	    reasonDesc: string;
	    count: number;
	    percentage: number;
	
	    static createFrom(source: any = {}) {
	        return new ReturnReasonStat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.reason = source["reason"];
	        this.reasonDesc = source["reasonDesc"];
	        this.count = source["count"];
	        this.percentage = source["percentage"];
	    }
	}
	export class SalesByAsin {
	    asin: string;
	    title: string;
	    sales: number;
	    units: number;
	    pageViews: number;
	    sessions: number;
	    conversionRate: number;
	    buyBoxPct: number;
	
	    static createFrom(source: any = {}) {
	        return new SalesByAsin(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.asin = source["asin"];
	        this.title = source["title"];
	        this.sales = source["sales"];
	        this.units = source["units"];
	        this.pageViews = source["pageViews"];
	        this.sessions = source["sessions"];
	        this.conversionRate = source["conversionRate"];
	        this.buyBoxPct = source["buyBoxPct"];
	    }
	}
	export class SearchTermStat {
	    searchTerm: string;
	    keywordText: string;
	    matchType: string;
	    impressions: number;
	    clicks: number;
	    cost: number;
	    sales7d: number;
	    purchases7d: number;
	    ctr: number;
	    cpc: number;
	    cvr: number;
	    acos: number;
	    optTag: string;
	    dataLatencyNote: string;
	
	    static createFrom(source: any = {}) {
	        return new SearchTermStat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.searchTerm = source["searchTerm"];
	        this.keywordText = source["keywordText"];
	        this.matchType = source["matchType"];
	        this.impressions = source["impressions"];
	        this.clicks = source["clicks"];
	        this.cost = source["cost"];
	        this.sales7d = source["sales7d"];
	        this.purchases7d = source["purchases7d"];
	        this.ctr = source["ctr"];
	        this.cpc = source["cpc"];
	        this.cvr = source["cvr"];
	        this.acos = source["acos"];
	        this.optTag = source["optTag"];
	        this.dataLatencyNote = source["dataLatencyNote"];
	    }
	}
	export class SeasonConfig {
	    accountId: number;
	    q1Factor: number;
	    q2Factor: number;
	    q3Factor: number;
	    q4Factor: number;
	    primeDayFactor: number;
	    autoApply: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SeasonConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.accountId = source["accountId"];
	        this.q1Factor = source["q1Factor"];
	        this.q2Factor = source["q2Factor"];
	        this.q3Factor = source["q3Factor"];
	        this.q4Factor = source["q4Factor"];
	        this.primeDayFactor = source["primeDayFactor"];
	        this.autoApply = source["autoApply"];
	    }
	}
	export class SettlementSummary {
	    settlementId: string;
	    startDate: string;
	    endDate: string;
	    depositDate: string;
	    totalAmount: number;
	    currency: string;
	
	    static createFrom(source: any = {}) {
	        return new SettlementSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.settlementId = source["settlementId"];
	        this.startDate = source["startDate"];
	        this.endDate = source["endDate"];
	        this.depositDate = source["depositDate"];
	        this.totalAmount = source["totalAmount"];
	        this.currency = source["currency"];
	    }
	}
	export class VATRate {
	    countryCode: string;
	    countryName: string;
	    marketplaceId: string;
	    standardRate: number;
	    reducedRate: number;
	    customRate?: number;
	    effectiveRate: number;
	    isCustom: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VATRate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.countryCode = source["countryCode"];
	        this.countryName = source["countryName"];
	        this.marketplaceId = source["marketplaceId"];
	        this.standardRate = source["standardRate"];
	        this.reducedRate = source["reducedRate"];
	        this.customRate = source["customRate"];
	        this.effectiveRate = source["effectiveRate"];
	        this.isCustom = source["isCustom"];
	    }
	}

}

