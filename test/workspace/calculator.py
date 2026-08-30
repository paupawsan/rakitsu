def add(a, b):
    return a + b

def subtract(a, b):
    return a - b

def multiply(a, b):
    return a * b

def divide(a, b):
    if b == 0:
        raise ValueError("Cannot divide by zero.")
    return a / b

def get_number(prompt):
    while True:
        try:
            return float(input(prompt))
        except ValueError:
            print("Invalid input. Please enter a numeric value.")

def get_operation():
    while True:
        op = input("Enter operation (+, -, *, /): ").strip()
        if op in ('+', '-', '*', '/'):
            return op
        print("Invalid operation. Please enter +, -, *, or /.")

def main():
    print("Simple Calculator")
    while True:
        a = get_number("Enter first number: ")
        b = get_number("Enter second number: ")
        op = get_operation()
        try:
            if op == '+':
                result = add(a, b)
            elif op == '-':
                result = subtract(a, b)
            elif op == '*':
                result = multiply(a, b)
            elif op == '/':
                result = divide(a, b)
            print(f"Result: {result}")
        except ValueError as e:
            print(f"Error: {e}")
        # Ask if user wants to continue
        cont = input("Do you want to perform another calculation? (y/n): ").strip().lower()
        if cont != 'y':
            print("Goodbye!")
            break

if __name__ == "__main__":
    main()