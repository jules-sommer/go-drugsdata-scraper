# Array of available quantities
quantities = [1, 2, 3, 4, 5, 10, 15, 20, 25, 30, 50]

# Unit price
unit_price = 8.50

# Function to calculate the price with discount based on quantity
def calculate_price(qty):
    if 5 <= qty <= 15:
        discount = 0.10  # 10% discount
    elif 15 < qty <= 25:
        discount = 0.15  # 15% discount
    elif 25 < qty <= 30:
        discount = 0.20  # 20% discount
    elif 30 < qty <= 50:
        discount = 0.25  # 25% discount
    else:
        discount = 0  # No discount

    return qty * unit_price * (1 - discount)

# Calculate prices for each quantity
prices = [calculate_price(qty) for qty in quantities]
print(prices)