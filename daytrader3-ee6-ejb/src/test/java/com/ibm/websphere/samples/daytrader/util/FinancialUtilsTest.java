package com.ibm.websphere.samples.daytrader.util;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

import java.math.BigDecimal;
import java.util.Arrays;
import java.util.Collections;
import java.util.Date;

import org.junit.jupiter.api.Test;

import com.ibm.websphere.samples.daytrader.HoldingDataBean;

class FinancialUtilsTest {

    private static BigDecimal bd(String value) {
        return new BigDecimal(value);
    }

    @Test
    void computeGainSubtractsOpenFromCurrentAtScaleTwo() {
        assertEquals(bd("150.25"), FinancialUtils.computeGain(bd("10150.25"), bd("10000.00")));
        assertEquals(bd("-24.95"), FinancialUtils.computeGain(bd("9975.05"), bd("10000.00")));
    }

    @Test
    void computeGainRejectsValuesThatNeedRounding() {
        // Legacy behaviour: setScale(2) without a rounding mode, so a third decimal place throws.
        assertThrows(ArithmeticException.class, () -> FinancialUtils.computeGain(bd("1.005"), bd("0.00")));
    }

    @Test
    void computeGainPercentIsRelativeToOpenBalance() {
        assertEquals(0, bd("10.00").compareTo(FinancialUtils.computeGainPercent(bd("110.00"), bd("100.00"))));
        assertEquals(0, bd("-25.00").compareTo(FinancialUtils.computeGainPercent(bd("75.00"), bd("100.00"))));
    }

    @Test
    void computeGainPercentIsZeroWhenOpenBalanceIsZero() {
        assertEquals(FinancialUtils.ZERO, FinancialUtils.computeGainPercent(bd("500.00"), bd("0.00")));
    }

    @Test
    void computeHoldingsTotalSumsPurchasePriceTimesQuantity() {
        HoldingDataBean a = new HoldingDataBean(1, 10.0, bd("145.06"), new Date(0), "s:1");
        HoldingDataBean b = new HoldingDataBean(2, 2.0, bd("0.50"), new Date(0), "s:2");
        assertEquals(bd("1451.60"), FinancialUtils.computeHoldingsTotal(Arrays.asList(a, b)));
    }

    @Test
    void computeHoldingsTotalHandlesNullAndEmpty() {
        assertEquals(bd("0.00"), FinancialUtils.computeHoldingsTotal(null));
        assertEquals(bd("0.00"), FinancialUtils.computeHoldingsTotal(Collections.emptyList()));
    }

    @Test
    void printGainHtmlUsesRedDownArrowForLossesAndGreenUpArrowOtherwise() {
        String loss = FinancialUtils.printGainHTML(bd("-1.5"));
        assertTrue(loss.startsWith("<FONT color=\"#ff0000\">-1.50</FONT>"), loss);
        assertTrue(loss.contains("arrowdown.gif"), loss);
        String gain = FinancialUtils.printGainHTML(bd("0"));
        assertTrue(gain.startsWith("<FONT color=\"#009900\">0.00</FONT>"), gain);
        assertTrue(gain.contains("arrowup.gif"), gain);
    }

    @Test
    void printGainPercentHtmlPrefixesPositiveValuesWithPlus() {
        assertTrue(FinancialUtils.printGainPercentHTML(bd("3.456")).startsWith("(<B><FONT color=\"#009900\">+3.46%"));
        assertTrue(FinancialUtils.printGainPercentHTML(bd("-3.454")).startsWith("(<B><FONT color=\"#ff0000\">-3.45%"));
    }

    @Test
    void printChangeHtmlRendersRawDouble() {
        assertTrue(FinancialUtils.printChangeHTML(-0.25).startsWith("<FONT color=\"#ff0000\">-0.25</FONT>"));
        assertTrue(FinancialUtils.printChangeHTML(1.0).startsWith("<FONT color=\"#009900\">1.0</FONT>"));
    }

    @Test
    void printQuoteLinkPointsAtQuotesAction() {
        assertEquals("<A href=\"app?action=quotes&symbols=s:7\">s:7</A>", FinancialUtils.printQuoteLink("s:7"));
    }
}
