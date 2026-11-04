package com.ibm.websphere.samples.daytrader;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.math.BigDecimal;

import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

class TradeConfigTest {

    private int runTimeMode;
    private int orderProcessingMode;
    private int workloadMix;
    private int webInterface;
    private int cachingType;
    private int maxUsers;
    private int maxQuotes;

    @BeforeEach
    void saveStaticConfig() {
        runTimeMode = TradeConfig.runTimeMode;
        orderProcessingMode = TradeConfig.orderProcessingMode;
        workloadMix = TradeConfig.workloadMix;
        webInterface = TradeConfig.webInterface;
        cachingType = TradeConfig.cachingType;
        maxUsers = TradeConfig.getMAX_USERS();
        maxQuotes = TradeConfig.getMAX_QUOTES();
    }

    @AfterEach
    void restoreStaticConfig() {
        TradeConfig.runTimeMode = runTimeMode;
        TradeConfig.orderProcessingMode = orderProcessingMode;
        TradeConfig.workloadMix = workloadMix;
        TradeConfig.webInterface = webInterface;
        TradeConfig.cachingType = cachingType;
        TradeConfig.setMAX_USERS(maxUsers);
        TradeConfig.setMAX_QUOTES(maxQuotes);
    }

    @Test
    void defaultsMatchTheShippedConfiguration() {
        assertEquals(TradeConfig.EJB3, TradeConfig.runTimeMode);
        assertEquals(TradeConfig.SYNCH, TradeConfig.orderProcessingMode);
        assertEquals(15000, TradeConfig.getMAX_USERS());
        assertEquals(10000, TradeConfig.getMAX_QUOTES());
        assertEquals("java:comp/env/jdbc/TradeDataSource", TradeConfig.DATASOURCE);
    }

    @Test
    void runTimeModeIsMatchedByDisplayNameIgnoringCaseAndWhitespace() {
        TradeConfig.setConfigParam("  RUNTIMEMODE ", "  direct (jdbc) ");
        assertEquals(TradeConfig.DIRECT, TradeConfig.runTimeMode);
        TradeConfig.setConfigParam("runTimeMode", "Session (EJB3) To Direct");
        assertEquals(TradeConfig.SESSION3, TradeConfig.runTimeMode);
    }

    @Test
    void unknownEnumValueLeavesSettingUnchanged() {
        TradeConfig.setConfigParam("orderProcessingMode", "Asynchronous_2-Phase");
        assertEquals(TradeConfig.ASYNCH_2PHASE, TradeConfig.orderProcessingMode);
        TradeConfig.setConfigParam("orderProcessingMode", "Eventually");
        assertEquals(TradeConfig.ASYNCH_2PHASE, TradeConfig.orderProcessingMode);
    }

    @Test
    void nullOrBlankParametersAreIgnored() {
        TradeConfig.setConfigParam(null, "Direct (JDBC)");
        TradeConfig.setConfigParam("   ", "Direct (JDBC)");
        TradeConfig.setConfigParam("runTimeMode", null);
        assertEquals(runTimeMode, TradeConfig.runTimeMode);
    }

    @Test
    void webInterfaceWorkloadMixAndCachingTypeAreParsed() {
        TradeConfig.setConfigParam("WebInterface", "JSP-Images");
        TradeConfig.setConfigParam("workloadMix", "High-Volume");
        TradeConfig.setConfigParam("CachingType", "Command Caching");
        assertEquals(TradeConfig.JSP_Images, TradeConfig.webInterface);
        assertEquals(TradeConfig.SCENARIOMIX_HIGHVOLUME, TradeConfig.workloadMix);
        assertEquals(TradeConfig.COMMAND_CACHING, TradeConfig.cachingType);
    }

    @Test
    void numericLimitsAreParsedAndBadNumbersAreIgnored() {
        TradeConfig.setConfigParam("maxUsers", " 200 ");
        TradeConfig.setConfigParam("maxQuotes", "50");
        assertEquals(200, TradeConfig.getMAX_USERS());
        assertEquals(50, TradeConfig.getMAX_QUOTES());
        TradeConfig.setConfigParam("maxUsers", "lots");
        assertEquals(200, TradeConfig.getMAX_USERS());
    }

    @Test
    void orderFeeAppliesToBuyAndSellOnly() {
        assertEquals(new BigDecimal("24.95"), TradeConfig.getOrderFee("buy"));
        assertEquals(new BigDecimal("24.95"), TradeConfig.getOrderFee("SELL"));
        assertEquals(0, BigDecimal.ZERO.compareTo(TradeConfig.getOrderFee("cash")));
    }

    @Test
    void nextUserIdWrapsAtMaxUsers() {
        TradeConfig.setMAX_USERS(3);
        boolean sawUid0 = false;
        for (int i = 0; i < 6; i++) {
            String userId = TradeConfig.nextUserID();
            assertTrue(userId.matches("uid:[0-2]"), userId);
            sawUid0 |= userId.equals("uid:0");
        }
        assertTrue(sawUid0);
    }

    @Test
    void randomSymbolsStayInsideTheQuotePopulation() {
        TradeConfig.setMAX_QUOTES(5);
        for (int i = 0; i < 200; i++) {
            int n = Integer.parseInt(TradeConfig.rndSymbol().substring(2));
            assertTrue(n >= 0 && n < 4, "s:" + n);
        }
    }

    @Test
    void priceChangeFactorStaysWithinTwentyPercent() {
        for (int i = 0; i < 500; i++) {
            BigDecimal factor = TradeConfig.getRandomPriceChangeFactor();
            assertTrue(factor.compareTo(new BigDecimal("0.80")) >= 0 && factor.compareTo(new BigDecimal("1.20")) <= 0,
                    factor.toPlainString());
            assertEquals(2, factor.scale());
        }
    }
}
